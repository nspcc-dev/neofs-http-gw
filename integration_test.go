package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	dockerContainer "github.com/docker/docker/api/types/container"
	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neofs-sdk-go/client"
	"github.com/nspcc-dev/neofs-sdk-go/container"
	"github.com/nspcc-dev/neofs-sdk-go/container/acl"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	"github.com/nspcc-dev/neofs-sdk-go/netmap"
	"github.com/nspcc-dev/neofs-sdk-go/object"
	oid "github.com/nspcc-dev/neofs-sdk-go/object/id"
	"github.com/nspcc-dev/neofs-sdk-go/pool"
	"github.com/nspcc-dev/neofs-sdk-go/user"
	"github.com/nspcc-dev/neofs-sdk-go/waiter"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type putResponse struct {
	CID string `json:"container_id"`
	OID string `json:"object_id"`
}

const (
	testContainerName = "friendly"
	testListenAddress = "localhost:8082"
	testHost          = "http://" + testListenAddress
)

var (
	tickEpoch = []string{
		"neo-go", "contract", "invokefunction", "--wallet-config", "/config/node-config.yaml",
		"-a", "NfgHwwTi3wHAS8aFAN243C5vGbkYDpqLHP", "--force", "-r", "http://localhost:30333",
		"707516630852f4179af43366917a36b9a78b93a5", "newEpoch", "int:10",
		"--", "NfgHwwTi3wHAS8aFAN243C5vGbkYDpqLHP:Global",
	}
)

func TestIntegration(t *testing.T) {
	versions := []string{"0.37.0", "0.38.0"}

	key, err := keys.NewPrivateKeyFromHex("1dd37fba80fec4e6a6f13fd708d8dcb3b29def768017052f6c930fa1c5d90bbb")
	require.NoError(t, err)

	signer := user.NewAutoIDSignerRFC6979(key.PrivateKey)
	ownerID := signer.UserID()

	for _, version := range versions {
		image := fmt.Sprintf("nspccdev/neofs-aio:%s", version)

		ctx, cancel2 := context.WithCancel(context.Background())

		aioContainer := createDockerContainer(ctx, t, image)
		server, cancel := runServer()
		clientPool := getPool(ctx, t, signer)
		CID, err := createContainer(ctx, t, clientPool, ownerID, signer)
		require.NoError(t, err, version)

		t.Run("simple put "+image, func(t *testing.T) { simplePut(ctx, t, clientPool, CID, signer) })
		t.Run("put with duplicate keys "+image, func(t *testing.T) { putWithDuplicateKeys(t, CID) })
		t.Run("simple get "+image, func(t *testing.T) { simpleGet(ctx, t, clientPool, ownerID, CID, signer) })
		t.Run("get by attribute "+image, func(t *testing.T) { getByAttr(ctx, t, clientPool, ownerID, CID, signer) })
		t.Run("get by attribute, not found "+image, func(t *testing.T) { getByAttrNotFound(t) })
		t.Run("get zip "+image, func(t *testing.T) { getZip(ctx, t, clientPool, ownerID, CID, signer) })

		cancel()
		server.Wait()
		err = aioContainer.Terminate(ctx)
		require.NoError(t, err)
		cancel2()
	}
}

func runServer() (App, context.CancelFunc) {
	cancelCtx, cancel := context.WithCancel(context.Background())

	v := getDefaultConfig()
	l, lvl := newLogger(v)
	application := newApp(cancelCtx, WithConfig(v), WithLogger(l, lvl))
	go application.Serve(cancelCtx)

	return application, cancel
}

func simplePut(ctx context.Context, t *testing.T, p *pool.Pool, CID cid.ID, signer user.Signer) {
	url := testHost + "/upload/" + testContainerName
	makePutRequestAndCheck(ctx, t, p, CID, url, signer)
}

func makePutRequestAndCheck(ctx context.Context, t *testing.T, p *pool.Pool, cnrID cid.ID, url string, signer user.Signer) {
	content := "content of file"
	keyAttr, valAttr := "User-Attribute", "user value"
	attributes := map[string]string{
		object.AttributeFileName:    "newFile.txt",
		object.AttributeContentType: "application/octet-stream",
		keyAttr:                     valAttr,
	}

	var buff bytes.Buffer
	w := multipart.NewWriter(&buff)
	fw, err := w.CreateFormFile("file", attributes[object.AttributeFileName])
	require.NoError(t, err)
	_, err = io.Copy(fw, bytes.NewBufferString(content))
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, url, &buff)
	require.NoError(t, err)
	request.Header.Set("Content-Type", w.FormDataContentType())
	request.Header.Set("X-Attribute-"+keyAttr, valAttr)

	resp, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() {
		err := resp.Body.Close()
		require.NoError(t, err)
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	if resp.StatusCode != http.StatusOK {
		fmt.Println(string(body))
	}
	require.Equal(t, http.StatusOK, resp.StatusCode)

	addr := &putResponse{}
	err = json.Unmarshal(body, addr)
	require.NoError(t, err)

	err = cnrID.DecodeString(addr.CID)
	require.NoError(t, err)

	var id oid.ID
	err = id.DecodeString(addr.OID)
	require.NoError(t, err)

	payload := bytes.NewBuffer(nil)

	var prm client.PrmObjectGet

	header, payloadReader, err := p.ObjectGetInit(ctx, cnrID, id, signer, prm)
	require.NoError(t, err)

	_, err = io.Copy(payload, payloadReader)
	require.NoError(t, err)

	require.Equal(t, content, payload.String())

	for _, attribute := range header.Attributes() {
		require.Equal(t, attributes[attribute.Key()], attribute.Value())
	}
}

func putWithDuplicateKeys(t *testing.T, CID cid.ID) {
	url := testHost + "/upload/" + CID.String()

	attr := "X-Attribute-User-Attribute"
	content := "content of file"
	valOne, valTwo := "first_value", "second_value"
	fileName := "newFile.txt"

	var buff bytes.Buffer
	w := multipart.NewWriter(&buff)
	fw, err := w.CreateFormFile("file", fileName)
	require.NoError(t, err)
	_, err = io.Copy(fw, bytes.NewBufferString(content))
	require.NoError(t, err)
	err = w.Close()
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, url, &buff)
	require.NoError(t, err)
	request.Header.Set("Content-Type", w.FormDataContentType())
	request.Header.Add(attr, valOne)
	request.Header.Add(attr, valTwo)

	resp, err := http.DefaultClient.Do(request)
	require.NoError(t, err)

	defer func() {
		err := resp.Body.Close()
		require.NoError(t, err)
	}()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "key duplication error: "+attr+"\n", string(body))
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func simpleGet(ctx context.Context, t *testing.T, clientPool *pool.Pool, ownerID user.ID, CID cid.ID, signer user.Signer) {
	content := "content of file"
	attributes := map[string]string{
		"some-attr": "some-get-value",
	}

	id := putObject(ctx, t, clientPool, ownerID, CID, content, attributes, signer)

	resp, err := http.Get(testHost + "/get/" + testContainerName + "/" + id.String())
	require.NoError(t, err)
	checkGetResponse(t, resp, content, attributes)
}

func checkGetResponse(t *testing.T, resp *http.Response, content string, attributes map[string]string) {
	defer func() {
		err := resp.Body.Close()
		require.NoError(t, err)
	}()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, content, string(data))

	for k, v := range attributes {
		require.Equal(t, v, resp.Header.Get("X-Attribute-"+k))
	}
}

func checkGetByAttrResponse(t *testing.T, resp *http.Response, content string, attributes map[string]string) {
	defer func() {
		err := resp.Body.Close()
		require.NoError(t, err)
	}()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, content, string(data))

	for k, v := range attributes {
		require.Equal(t, v, resp.Header.Get(k))
	}
}

func getByAttr(ctx context.Context, t *testing.T, clientPool *pool.Pool, ownerID user.ID, CID cid.ID, signer user.Signer) {
	keyAttr, valAttr := "some-attr", "some-get-by-attr-value"
	content := "content of file"
	attributes := map[string]string{keyAttr: valAttr}

	id := putObject(ctx, t, clientPool, ownerID, CID, content, attributes, signer)

	expectedAttr := map[string]string{
		"X-Attribute-" + keyAttr: valAttr,
		"x-object-id":            id.String(),
		"x-container-id":         CID.String(),
	}

	resp, err := http.Get(testHost + "/get_by_attribute/" + testContainerName + "/" + keyAttr + "/" + valAttr)
	require.NoError(t, err)
	checkGetByAttrResponse(t, resp, content, expectedAttr)
}

func getByAttrNotFound(t *testing.T) {
	keyAttr, valAttr := "some-attr-no", "some-get-by-attr-value-no"

	resp, err := http.Get(testHost + "/get_by_attribute/" + testContainerName + "/" + keyAttr + "/" + valAttr)

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.NoError(t, err)
}

func getZip(ctx context.Context, t *testing.T, clientPool *pool.Pool, ownerID user.ID, CID cid.ID, signer user.Signer) {
	names := []string{"zipfolder/dir/name1.txt", "zipfolder/name2.txt"}
	contents := []string{"content of file1", "content of file2"}
	attributes1 := map[string]string{object.AttributeFilePath: names[0]}
	attributes2 := map[string]string{object.AttributeFilePath: names[1]}

	putObject(ctx, t, clientPool, ownerID, CID, contents[0], attributes1, signer)
	putObject(ctx, t, clientPool, ownerID, CID, contents[1], attributes2, signer)

	baseURL := testHost + "/zip/" + testContainerName
	makeZipTest(t, baseURL, names, contents)
}

func makeZipTest(t *testing.T, baseURL string, names, contents []string) {
	url := baseURL + "/zipfolder"
	makeZipRequest(t, url, names, contents)

	// check nested folder
	url = baseURL + "/zipfolder/dir"
	makeZipRequest(t, url, names[:1], contents[:1])
}

func makeZipRequest(t *testing.T, url string, names, contents []string) {
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() {
		err := resp.Body.Close()
		require.NoError(t, err)
	}()

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	checkZip(t, data, int64(len(data)), names, contents)
}

func checkZip(t *testing.T, data []byte, length int64, names, contents []string) {
	readerAt := bytes.NewReader(data)

	zipReader, err := zip.NewReader(readerAt, length)
	require.NoError(t, err)

	require.Equal(t, len(names), len(zipReader.File))

	sort.Slice(zipReader.File, func(i, j int) bool {
		return zipReader.File[i].FileHeader.Name < zipReader.File[j].FileHeader.Name
	})

	for i, f := range zipReader.File {
		require.Equal(t, names[i], f.FileHeader.Name)

		rc, err := f.Open()
		require.NoError(t, err)

		all, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.Equal(t, contents[i], string(all))

		err = rc.Close()
		require.NoError(t, err)
	}
}

func createDockerContainer(ctx context.Context, t *testing.T, image string) testcontainers.Container {
	req := testcontainers.ContainerRequest{
		Image:      image,
		WaitingFor: wait.NewLogStrategy("aio container started").WithStartupTimeout(30 * time.Second),
		Name:       "http-gate-tests-aio",
		Hostname:   "http-gate-tests-aio",
		HostConfigModifier: func(hostConfig *dockerContainer.HostConfig) {
			hostConfig.NetworkMode = "host"
		},
	}
	aioC, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	// Have to wait this time. Required for new tick event processing.
	// Should be removed after fix epochs in AIO start.
	<-time.After(3 * time.Second)

	_, _, err = aioC.Exec(ctx, tickEpoch)
	require.NoError(t, err)

	<-time.After(3 * time.Second)

	return aioC
}

func getDefaultConfig() *viper.Viper {
	v := settings()
	v.SetDefault(cfgPeers+".0.address", "localhost:8080")
	v.SetDefault(cfgPeers+".0.weight", 1)
	v.SetDefault(cfgPeers+".0.priority", 1)

	v.SetDefault(cfgRPCEndpoint, "http://localhost:30333")
	v.SetDefault("server.0.address", testListenAddress)

	return v
}

func getPool(ctx context.Context, t *testing.T, signer user.Signer) *pool.Pool {
	var prm pool.InitParameters
	prm.SetSigner(signer)
	prm.SetNodeDialTimeout(5 * time.Second)
	prm.AddNode(pool.NewNodeParam(1, "localhost:8080", 1))

	clientPool, err := pool.NewPool(prm)
	require.NoError(t, err)

	err = clientPool.Dial(ctx)
	require.NoError(t, err)
	return clientPool
}

func createContainer(ctx context.Context, t *testing.T, clientPool *pool.Pool, ownerID user.ID, signer user.Signer) (cid.ID, error) {
	var policy netmap.PlacementPolicy
	err := policy.DecodeString("REP 1")
	require.NoError(t, err)

	var cnr container.Container
	cnr.Init()
	cnr.SetPlacementPolicy(policy)
	cnr.SetBasicACL(acl.PublicRWExtended)
	cnr.SetOwner(ownerID)

	cnr.SetCreationTime(time.Now())

	var domain container.Domain
	domain.SetName(testContainerName)
	cnr.WriteDomain(domain)

	w := waiter.NewContainerPutWaiter(clientPool, waiter.DefaultPollInterval)

	var prm client.PrmContainerPut
	CID, err := w.ContainerPut(ctx, cnr, signer, prm)
	if err != nil {
		return cid.ID{}, err
	}
	fmt.Println(CID.String())

	return CID, err
}

func putObject(ctx context.Context, t *testing.T, clientPool *pool.Pool, ownerID user.ID, CID cid.ID, content string, attributes map[string]string, signer user.Signer) oid.ID {
	var obj object.Object
	obj.SetContainerID(CID)
	obj.SetOwnerID(&ownerID)

	var attrs []object.Attribute
	for key, val := range attributes {
		attr := object.NewAttribute()
		attr.SetKey(key)
		attr.SetValue(val)
		attrs = append(attrs, *attr)
	}
	obj.SetAttributes(attrs...)

	var prm client.PrmObjectPutInit
	writer, err := clientPool.ObjectPutInit(ctx, obj, signer, prm)
	require.NoError(t, err)

	data := strings.NewReader(content)
	chunk := make([]byte, 2048)
	_, err = io.CopyBuffer(writer, data, chunk)
	require.NoError(t, err)

	err = writer.Close()
	require.NoError(t, err)

	return writer.GetResult().StoredObjectID()
}
