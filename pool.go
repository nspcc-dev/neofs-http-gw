package main

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"math/rand"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/nspcc-dev/neofs-api/service"
	"github.com/nspcc-dev/neofs-api/state"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"
)

type (
	node struct {
		address string
		weight  uint32
		conn    *grpc.ClientConn
	}

	Pool struct {
		log *zap.Logger

		connectTimeout time.Duration
		opts           keepalive.ClientParameters

		cur *grpc.ClientConn

		*sync.RWMutex
		nodes []*node
		keys  []uint32
		conns map[uint32][]*grpc.ClientConn
	}
)

var (
	errEmptyConnection      = errors.New("empty connection")
	errNoHealthyConnections = errors.New("no active connections")
)

func newPool(ctx context.Context, l *zap.Logger, v *viper.Viper) *Pool {
	p := &Pool{
		log:     l,
		RWMutex: new(sync.RWMutex),
		keys:    make([]uint32, 0),
		nodes:   make([]*node, 0),
		conns:   make(map[uint32][]*grpc.ClientConn),

		// fill with defaults:
		connectTimeout: time.Second * 15,
		opts: keepalive.ClientParameters{
			Time:                time.Second * 10,
			Timeout:             time.Minute * 5,
			PermitWithoutStream: true,
		},
	}
	buf := make([]byte, 8)
	if _, err := crand.Read(buf); err != nil {
		l.Panic("could not read seed", zap.Error(err))
	}

	seed := binary.BigEndian.Uint64(buf)
	rand.Seed(int64(seed))
	l.Info("used random seed", zap.Uint64("seed", seed))

	if val := v.GetDuration("connect_timeout"); val > 0 {
		p.connectTimeout = val
	}

	if val := v.GetDuration("keepalive.time"); val > 0 {
		p.opts.Time = val
	}

	if val := v.GetDuration("keepalive.timeout"); val > 0 {
		p.opts.Timeout = val
	}

	if v.IsSet("keepalive.permit_without_stream") {
		p.opts.PermitWithoutStream = v.GetBool("keepalive.permit_without_stream")
	}

	for i := 0; ; i++ {
		key := "peers." + strconv.Itoa(i) + "."
		address := v.GetString(key + "address")
		weight := v.GetFloat64(key + "weight")

		if address == "" {
			l.Warn("skip, empty address")
			break
		}

		p.nodes = append(p.nodes, &node{
			address: address,
			weight:  uint32(weight * 100),
		})

		l.Info("add new peer",
			zap.String("address", p.nodes[i].address),
			zap.Uint32("weight", p.nodes[i].weight))
	}

	p.reBalance(ctx)

	cur, err := p.getConnection(ctx)
	if err != nil {
		l.Panic("could get connection", zap.Error(err))
	}

	p.cur = cur

	return p
}

func (p *Pool) close() {
	p.Lock()
	defer p.Unlock()

	for i := range p.nodes {
		if p.nodes[i] == nil || p.nodes[i].conn == nil {
			continue
		}

		p.log.Warn("close connection",
			zap.String("address", p.nodes[i].address),
			zap.Error(p.nodes[i].conn.Close()))
	}
}

func (p *Pool) reBalance(ctx context.Context) {
	p.Lock()
	defer p.Unlock()

	keys := make(map[uint32]struct{})

	p.log.Info("re-balancing connections")

	for i := range p.nodes {
		var (
			idx    = -1
			exists bool
			err    error
			start  = time.Now()
			conn   = p.nodes[i].conn
			weight = p.nodes[i].weight
		)

		if conn == nil {
			p.log.Warn("empty connection, try to connect",
				zap.String("address", p.nodes[i].address))

			ctx, cancel := context.WithTimeout(ctx, p.connectTimeout)
			conn, err = grpc.DialContext(ctx, p.nodes[i].address,
				grpc.WithBlock(),
				grpc.WithInsecure(),
				grpc.WithKeepaliveParams(p.opts))
			cancel()

			if err != nil || conn == nil {
				p.log.Warn("skip, could not connect to node",
					zap.String("address", p.nodes[i].address),
					zap.Duration("elapsed", time.Since(start)),
					zap.Error(err))
				continue
			}

			p.nodes[i].conn = conn
			p.log.Info("connected to node", zap.String("address", p.nodes[i].address))
		}

		for j := range p.conns[weight] {
			if p.conns[weight][j] == conn {
				idx = j
				exists = true
				break
			}
		}

		// if something wrong with connection (bad state or unhealthy), try to close it and remove
		if err = isAlive(ctx, p.log, conn); err != nil {
			p.log.Warn("connection not alive",
				zap.String("address", p.nodes[i].address),
				zap.Error(err))

			if exists {
				// remove from connections
				p.conns[weight] = append(p.conns[weight][:idx], p.conns[weight][idx+1:]...)
			}

			if err = conn.Close(); err != nil {
				p.log.Warn("could not close bad connection",
					zap.String("address", p.nodes[i].address),
					zap.Error(err))
			}

			if p.nodes[i].conn != nil {
				p.nodes[i].conn = nil
			}
			continue
		}

		keys[weight] = struct{}{}

		if !exists {
			p.conns[weight] = append(p.conns[weight], conn)
		}
	}

	p.keys = p.keys[:0]
	for w := range keys {
		p.keys = append(p.keys, w)
	}

	sort.Slice(p.keys, func(i, j int) bool {
		return p.keys[i] > p.keys[j]
	})
}

func (p *Pool) getConnection(ctx context.Context) (*grpc.ClientConn, error) {
	p.RLock()
	defer p.RUnlock()

	if err := isAlive(ctx, p.log, p.cur); err == nil {
		return p.cur, nil
	}

	for _, w := range p.keys {
		switch ln := len(p.conns[w]); ln {
		case 0:
			continue
		case 1:
			p.cur = p.conns[w][0]
			return p.cur, nil
		default: // > 1
			i := rand.Intn(ln)
			p.cur = p.conns[w][i]
			return p.cur, nil
		}
	}

	return nil, errNoHealthyConnections
}

func isAlive(ctx context.Context, log *zap.Logger, cur *grpc.ClientConn) error {
	if cur == nil {
		return errEmptyConnection
	}

	switch st := cur.GetState(); st {
	case connectivity.Idle, connectivity.Ready, connectivity.Connecting:
		req := new(state.HealthRequest)
		req.SetTTL(service.NonForwardingTTL)

		res, err := state.NewStatusClient(cur).HealthCheck(ctx, req)
		if err != nil {
			log.Warn("could not fetch health-check", zap.Error(err))

			return err
		} else if !res.Healthy {
			return errors.New(res.Status)
		}

		return nil
	default:
		return errors.New(st.String())
	}
}
