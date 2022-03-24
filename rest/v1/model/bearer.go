package model

import (
	"encoding/hex"
	"fmt"

	"github.com/nspcc-dev/neofs-sdk-go/eacl"
	"github.com/nspcc-dev/neofs-sdk-go/token"
)

type (
	// Bearer is model for request body to form bearer token to sing.
	Bearer struct {
		Records []Record `json:"records"`
	}

	// Record is json-friendly eacl.Record.
	Record struct {
		Operation Operation `json:"operation"`
		Action    Action    `json:"action"`
		Filters   []Filter  `json:"filters"`
		Targets   []Target  `json:"targets"`
	}

	// Filter is json-friendly eacl.Filter.
	Filter struct {
		HeaderType HeaderType `json:"headerType"`
		MatchType  MatchType  `json:"matchType"`
		Key        string     `json:"key"`
		Value      string     `json:"value"`
	}

	// Target is json-friendly eacl.Target.
	Target struct {
		Role Role     `json:"role"`
		Keys []string `json:"keys"`
	}
)

// Operation is json-friendly eacl.Operation.
type Operation string

const (
	// OperationUnknown is an operation that maps to eacl.OperationUnknown.
	OperationUnknown Operation = ""

	// OperationGet is an operation that maps to eacl.OperationGet.
	OperationGet Operation = "GET"

	// OperationHead is an operation that maps to eacl.OperationHead.
	OperationHead Operation = "HEAD"

	// OperationPut is an operation that maps to eacl.OperationPut.
	OperationPut Operation = "PUT"

	// OperationDelete is an operation that maps to eacl.OperationDelete.
	OperationDelete Operation = "DELETE"

	// OperationSearch is an operation that maps to eacl.OperationSearch.
	OperationSearch Operation = "SEARCH"

	// OperationRange is an operation that maps to eacl.OperationRange.
	OperationRange Operation = "RANGE"

	// OperationRangeHash is an operation that maps to eacl.OperationRangeHash.
	OperationRangeHash Operation = "RANGE_HASH"
)

// Action is json-friendly eacl.Action.
type Action string

const (
	// ActionUnknown is action that maps to eacl.ActionUnknown.
	ActionUnknown Action = ""

	// ActionAllow is action that maps to eacl.ActionAllow.
	ActionAllow Action = "ALLOW"

	// ActionDeny is action that maps to eacl.ActionDeny.
	ActionDeny Action = "DENY"
)

// HeaderType is json-friendly eacl.FilterHeaderType.
type HeaderType string

const (
	// HeaderTypeUnknown is a header type that maps to eacl.HeaderTypeUnknown.
	HeaderTypeUnknown HeaderType = ""

	// HeaderTypeRequest is a header type that maps to eacl.HeaderTypeRequest.
	HeaderTypeRequest HeaderType = "REQUEST"

	// HeaderTypeObject is a header type that maps to eacl.HeaderTypeObject.
	HeaderTypeObject HeaderType = "OBJECT"

	// HeaderTypeService is a header type that maps to eacl.HeaderTypeService.
	HeaderTypeService HeaderType = "SERVICE"
)

// MatchType is json-friendly eacl.Match.
type MatchType string

const (
	// MatchTypeUnknown is a match type that maps to eacl.MatchUnknown.
	MatchTypeUnknown MatchType = ""

	// MatchTypeStringEqual is a match type that maps to eacl.MatchStringEqual.
	MatchTypeStringEqual MatchType = "STRING_EQUAL"

	// MatchTypeStringNotEqual is a match type that maps to eacl.MatchStringNotEqual.
	MatchTypeStringNotEqual MatchType = "STRING_NOT_EQUAL"
)

// Role is json-friendly eacl.Role.
type Role string

const (
	// RoleUnknown is a role that maps to eacl.RoleUnknown.
	RoleUnknown Role = ""

	// RoleUser is a role that maps to eacl.RoleUser.
	RoleUser Role = "USER"

	// RoleSystem is a role that maps to eacl.RoleSystem.
	RoleSystem Role = "SYSTEM"

	// RoleOthers is a role that maps to eacl.RoleOthers.
	RoleOthers Role = "OTHERS"
)

// ToNative converts Action to appropriate eacl.Action.
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

// ToNative converts Operation to appropriate eacl.Operation.
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

// ToNative converts HeaderType to appropriate eacl.FilterHeaderType.
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

// ToNative converts MatchType to appropriate eacl.Match.
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

// ToNative converts Role to appropriate eacl.Role.
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

// ToNative converts Record to appropriate eacl.Record.
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

// ToNative converts Target to appropriate eacl.Target.
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

// ToNative converts Bearer to appropriate token.BearerToken.
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
