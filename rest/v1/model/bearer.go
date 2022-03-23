package model

import (
	"encoding/hex"
	"fmt"

	"github.com/nspcc-dev/neofs-sdk-go/eacl"
	"github.com/nspcc-dev/neofs-sdk-go/token"
)

type (
	Bearer struct {
		Records []Record `json:"records"`
	}

	Record struct {
		Operation Operation `json:"operation"`
		Action    Action    `json:"action"`
		Filters   []Filter  `json:"filters"`
		Targets   []Target  `json:"targets"`
	}

	Filter struct {
		HeaderType HeaderType `json:"headerType"`
		MatchType  MatchType  `json:"matchType"`
		Key        string     `json:"key"`
		Value      string     `json:"value"`
	}

	Target struct {
		Role Role     `json:"role"`
		Keys []string `json:"keys"`
	}
)

type Operation string

const (
	OperationUnknown   Operation = ""
	OperationGet                 = "GET"
	OperationHead                = "HEAD"
	OperationPut                 = "PUT"
	OperationDelete              = "DELETE"
	OperationSearch              = "SEARCH"
	OperationRange               = "RANGE"
	OperationRangeHash           = "RANGE_HASH"
)

type Action string

const (
	ActionUnknown Action = ""
	ActionAllow          = "ALLOW"
	ActionDeny           = "DENY"
)

type HeaderType string

const (
	HeaderTypeUnknown HeaderType = ""
	HeaderTypeRequest            = "REQUEST"
	HeaderTypeObject             = "OBJECT"
	HeaderTypeService            = "SERVICE"
)

type MatchType string

const (
	MatchTypeUnknown        MatchType = ""
	MatchTypeStringEqual              = "STRING_EQUAL"
	MatchTypeStringNotEqual           = "STRING_NOT_EQUAL"
)

type Role string

const (
	RoleUnknown Role = ""
	RoleUser         = "USER"
	RoleSystem       = "SYSTEM"
	RoleOthers       = "OTHERS"
)

func (a Action) ToNative() (eacl.Action, error) {
	switch a {
	case ActionAllow:
		return eacl.ActionAllow, nil
	case ActionDeny:
		return eacl.ActionDeny, nil
	default:
		return eacl.ActionUnknown, fmt.Errorf("unsupported action type: '%s'", a)
	}
}

func (o Operation) ToNative() (eacl.Operation, error) {
	switch o {
	case OperationGet:
		return eacl.OperationGet, nil
	case OperationHead:
		return eacl.OperationHead, nil
	case OperationPut:
		return eacl.OperationPut, nil
	case OperationDelete:
		return eacl.OperationDelete, nil
	case OperationSearch:
		return eacl.OperationSearch, nil
	case OperationRange:
		return eacl.OperationRange, nil
	case OperationRangeHash:
		return eacl.OperationRangeHash, nil
	default:
		return eacl.OperationUnknown, fmt.Errorf("unsupported operation type: '%s'", o)
	}
}

func (h HeaderType) ToNative() (eacl.FilterHeaderType, error) {
	switch h {
	case HeaderTypeObject:
		return eacl.HeaderFromObject, nil
	case HeaderTypeRequest:
		return eacl.HeaderFromRequest, nil
	case HeaderTypeService:
		return eacl.HeaderFromService, nil
	default:
		return eacl.HeaderTypeUnknown, fmt.Errorf("unsupported header type: '%s'", h)
	}
}

func (h MatchType) ToNative() (eacl.Match, error) {
	switch h {
	case MatchTypeStringEqual:
		return eacl.MatchStringEqual, nil
	case MatchTypeStringNotEqual:
		return eacl.MatchStringNotEqual, nil
	default:
		return eacl.MatchUnknown, fmt.Errorf("unsupported match type: '%s'", h)
	}
}

func (r Role) ToNative() (eacl.Role, error) {
	switch r {
	case RoleUser:
		return eacl.RoleUser, nil
	case RoleSystem:
		return eacl.RoleSystem, nil
	case RoleOthers:
		return eacl.RoleOthers, nil
	default:
		return eacl.RoleUnknown, fmt.Errorf("unsupported role type: '%s'", r)
	}
}

func (r *Record) ToNative() (*eacl.Record, error) {
	var record eacl.Record

	action, err := r.Action.ToNative()
	if err != nil {
		return nil, err
	}
	record.SetAction(action)

	operation, err := r.Operation.ToNative()
	if err != nil {
		return nil, err
	}
	record.SetOperation(operation)

	for _, filter := range r.Filters {
		headerType, err := filter.HeaderType.ToNative()
		if err != nil {
			return nil, err
		}
		matchType, err := filter.MatchType.ToNative()
		if err != nil {
			return nil, err
		}
		record.AddFilter(headerType, matchType, filter.Key, filter.Value)
	}

	targets := make([]eacl.Target, len(r.Targets))
	for i, target := range r.Targets {
		trgt, err := target.ToNative()
		if err != nil {
			return nil, err
		}
		targets[i] = *trgt
	}
	record.SetTargets(targets...)

	return &record, nil
}

func (t *Target) ToNative() (*eacl.Target, error) {
	var target eacl.Target

	role, err := t.Role.ToNative()
	if err != nil {
		return nil, err
	}
	target.SetRole(role)

	keys := make([][]byte, len(t.Keys))
	for i, key := range t.Keys {
		binaryKey, err := hex.DecodeString(key)
		if err != nil {
			return nil, fmt.Errorf("couldn't decode target key: %w", err)
		}
		keys[i] = binaryKey
	}
	target.SetBinaryKeys(keys)

	return &target, nil
}

func (b *Bearer) ToNative() (*token.BearerToken, error) {
	var btoken token.BearerToken
	var table eacl.Table

	for _, rec := range b.Records {
		record, err := rec.ToNative()
		if err != nil {
			return nil, fmt.Errorf("couldn't transform record to native: %w", err)
		}
		table.AddRecord(record)
	}

	btoken.SetEACLTable(&table)

	return &btoken, nil
}
