//-----------------------------------------------------------------------------
// Copyright (c) 2022 Detlef Stern
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
	"log"
	"net/http"
	"net/url"
	"testing"

	"zettelstore.de/c/api"
	"zettelstore.de/c/client"
	"zettelstore.de/c/zjson"
)

func TestZettelList(t *testing.T) {
	c := getClient()
	_, err := c.ListZettel(context.Background(), nil)
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

func TestGetZJSONZettel(t *testing.T) {
	c := getClient()
	data, err := c.GetEvaluatedZJSON(context.Background(), api.ZidDefaultHome, api.PartContent, true)
	if err != nil {
		t.Error(err)
		return
	}
	if data == nil {
		t.Error("No data")
	}
	var v vis
	zjson.WalkBlock(&v, data.(zjson.Array), -1)
	// t.Error("Argh")
}

type vis struct{}

func (*vis) BlockArray(a zjson.Array, pos int) zjson.CloseFunc {
	log.Println("SBLO", pos, a)
	return nil
}
func (*vis) InlineArray(a zjson.Array, pos int) zjson.CloseFunc {
	log.Println("SINL", pos, a)
	return nil
}
func (*vis) ItemArray(a zjson.Array, pos int) zjson.CloseFunc {
	log.Println("SITE", pos, a)
	return nil
}
func (*vis) BlockObject(t string, obj zjson.Object, pos int) (bool, zjson.CloseFunc) {
	log.Println("BOBJ", pos, t, obj)
	return true, nil
}
func (*vis) InlineObject(t string, obj zjson.Object, pos int) (bool, zjson.CloseFunc) {
	log.Println("IOBJ", pos, t, obj)
	return true, nil
}
func (*vis) Unexpected(val zjson.Value, pos int, exp string) { log.Println("Expect", pos, exp, val) }

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
