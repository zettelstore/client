//-----------------------------------------------------------------------------
// Copyright (c) 2022-2023 Detlef Stern
//
// This file is part of zettelstore-client.
//
// Zettelstore client is licensed under the latest version of the EUPL
// (European Union Public License). Please see file LICENSE.txt for your rights
// and obligations under this license.
//-----------------------------------------------------------------------------

package client_test

import (
	"context"
	"flag"
	"net/http"
	"net/url"
	"testing"

	"codeberg.org/t73fde/sxpf"
	"zettelstore.de/c/api"
	"zettelstore.de/c/client"
)

func TestZettelList(t *testing.T) {
	c := getClient()
	_, err := c.ListZettel(context.Background(), "")
	if err != nil {
		t.Error(err)
		return
	}
}

func TestGetProtectedZettel(t *testing.T) {
	c := getClient()
	_, err := c.GetZettel(context.Background(), api.ZidStartupConfiguration, api.PartZettel)
	if err != nil {
		if cErr, ok := err.(*client.Error); ok && cErr.StatusCode == http.StatusForbidden {
			return
		} else {
			t.Error(err)
		}
		return
	}
}

func TestGetSexprZettel(t *testing.T) {
	c := getClient()
	value, err := c.GetEvaluatedSexpr(context.Background(), api.ZidDefaultHome, api.PartContent)
	if err != nil {
		t.Error(err)
		return
	}
	if value == nil {
		t.Error("No data")
	}
	var env testEnv
	env.t = t
	res, err := sxpf.Eval(&env, value)
	if err != nil {
		t.Error(res, err)
	}
}

type testEnv struct{ t *testing.T }

func noneFn(sxpf.Environment, *sxpf.Pair, int) (sxpf.Value, error) { return sxpf.Nil(), nil }
func (*testEnv) LookupForm(*sxpf.Symbol) (sxpf.Form, error) {
	return sxpf.NewBuiltin("none", false, 0, -1, noneFn), nil
}
func (*testEnv) EvalSymbol(sym *sxpf.Symbol) (sxpf.Value, error) { return sym, nil }
func (*testEnv) EvalOther(val sxpf.Value) (sxpf.Value, error)    { return val, nil }
func (te *testEnv) EvalPair(p *sxpf.Pair) (sxpf.Value, error)    { return sxpf.EvalCallOrSeq(te, p) }

var baseURL string

func init() {
	flag.StringVar(&baseURL, "base-url", "http://localhost:23123/", "Base URL")
}

func getClient() *client.Client {
	u, err := url.Parse(baseURL)
	if err != nil {
		panic(err)
	}
	return client.NewClient(u)
}

func TestBase(t *testing.T) {
	exp := baseURL
	got := getClient().Base()
	if exp != got {
		t.Errorf("Base: expected=%q, but got=%q", exp, got)
	}
}

// TestMain controls whether client API tests should run or not.
func TestMain(m *testing.M) {
	flag.Parse()
	if baseURL != "" {
		m.Run()
	}
}
