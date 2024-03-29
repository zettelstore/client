//-----------------------------------------------------------------------------
// Copyright (c) 2020-present Detlef Stern
//
// This file is part of zettelstore-client.
//
// Zettelstore client is licensed under the latest version of the EUPL
// (European Union Public License). Please see file LICENSE.txt for your rights
// and obligations under this license.
//-----------------------------------------------------------------------------

package api

import (
	"net/url"
	"strings"
)

type urlQuery struct{ key, val string }

// URLBuilder should be used to create zettelstore URLs.
type URLBuilder struct {
	prefix   string
	key      byte
	rawLocal string
	path     []string
	query    []urlQuery
	fragment string
}

// NewURLBuilder creates a new URL builder with the given prefix and key.
func NewURLBuilder(prefix string, key byte) *URLBuilder {
	return &URLBuilder{prefix: prefix, key: key}
}

// Clone an URLBuilder
func (ub *URLBuilder) Clone() *URLBuilder {
	cpy := new(URLBuilder)
	cpy.key = ub.key
	if len(ub.path) > 0 {
		cpy.path = make([]string, 0, len(ub.path))
		cpy.path = append(cpy.path, ub.path...)
	}
	if len(ub.query) > 0 {
		cpy.query = make([]urlQuery, 0, len(ub.query))
		cpy.query = append(cpy.query, ub.query...)
	}
	cpy.fragment = ub.fragment
	return cpy
}

// SetRawLocal sets everything that follows the prefix / key.
func (ub *URLBuilder) SetRawLocal(rawLocal string) *URLBuilder {
	for len(rawLocal) > 0 && rawLocal[0] == '/' {
		rawLocal = rawLocal[1:]
	}
	ub.rawLocal = rawLocal
	ub.path = nil
	ub.query = nil
	ub.fragment = ""
	return ub
}

// SetZid sets the zettel identifier.
func (ub *URLBuilder) SetZid(zid ZettelID) *URLBuilder {
	if len(ub.path) > 0 {
		panic("Cannot add Zid")
	}
	ub.rawLocal = ""
	ub.path = append(ub.path, string(zid))
	return ub
}

// AppendPath adds a new path element
func (ub *URLBuilder) AppendPath(p string) *URLBuilder {
	ub.rawLocal = ""
	for len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	if p != "" {
		ub.path = append(ub.path, p)
	}
	return ub
}

// AppendKVQuery adds a new key/value query parameter
func (ub *URLBuilder) AppendKVQuery(key, value string) *URLBuilder {
	ub.rawLocal = ""
	ub.query = append(ub.query, urlQuery{key, value})
	return ub
}

// AppendQuery adds a new query
func (ub *URLBuilder) AppendQuery(value string) *URLBuilder {
	ub.rawLocal = ""
	ub.query = append(ub.query, urlQuery{QueryKeyQuery, value})
	return ub
}

// ClearQuery removes all query parameters.
func (ub *URLBuilder) ClearQuery() *URLBuilder {
	ub.rawLocal = ""
	ub.query = nil
	ub.fragment = ""
	return ub
}

// SetFragment stores the fragment
func (ub *URLBuilder) SetFragment(s string) *URLBuilder {
	ub.rawLocal = ""
	ub.fragment = s
	return ub
}

// String produces a string value.
func (ub *URLBuilder) String() string {
	var sb strings.Builder

	sb.WriteString(ub.prefix)
	if ub.key != '/' {
		sb.WriteByte(ub.key)
	}
	if ub.rawLocal != "" {
		sb.WriteString(ub.rawLocal)
		return sb.String()
	}
	for i, p := range ub.path {
		if i > 0 || ub.key != '/' {
			sb.WriteByte('/')
		}
		sb.WriteString(url.PathEscape(p))
	}
	if len(ub.fragment) > 0 {
		sb.WriteByte('#')
		sb.WriteString(ub.fragment)
	}
	for i, q := range ub.query {
		if i == 0 {
			sb.WriteByte('?')
		} else {
			sb.WriteByte('&')
		}
		sb.WriteString(q.key)
		if val := q.val; val != "" {
			sb.WriteByte('=')
			sb.WriteString(url.QueryEscape(val))
		}
	}
	return sb.String()
}
