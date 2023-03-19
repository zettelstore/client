//-----------------------------------------------------------------------------
// Copyright (c) 2022-present Detlef Stern
//
// This file is part of zettelstore-client.
//
// Zettelstore client is licensed under the latest version of the EUPL
// (European Union Public License). Please see file LICENSE.txt for your rights
// and obligations under this license.
//-----------------------------------------------------------------------------

package text_test

import (
	"strings"
	"testing"

	"codeberg.org/t73fde/sxpf"
	"codeberg.org/t73fde/sxpf/reader"
	"zettelstore.de/c/text"
)

func TestSexprText(t *testing.T) {
	testcases := []struct {
		src string
		exp string
	}{
		{"()", ""},
		{`(INLINE (TEXT "a"))`, "a"},
		{`(INLINE (SPACE "a"))`, " "},
	}
	for i, tc := range testcases {
		sval, err := reader.MakeReader(strings.NewReader(tc.src)).Read()
		if err != nil {
			t.Error(err)
			continue
		}
		seq, ok := sval.(*sxpf.List)
		if !ok {
			t.Errorf("%d: not a list: %v", i, sval)
		}
		got := text.EvaluateInlineString(seq)
		if got != tc.exp {
			t.Errorf("%d: EncodeBlock(%q) == %q, but got %q", i, tc.src, tc.exp, got)
		}
	}
}
