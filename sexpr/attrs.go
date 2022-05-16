//-----------------------------------------------------------------------------
// Copyright (c) 2022 Detlef Stern
//
// This file is part of zettelstore-client.
//
// Zettelstore client is licensed under the latest version of the EUPL
// (European Union Public License). Please see file LICENSE.txt for your rights
// and obligations under this license.
//-----------------------------------------------------------------------------

package sexpr

import (
	"github.com/t73fde/sxpf"
	"zettelstore.de/c/attrs"
)

// GetAttributes traverses a s-expression list and returns an attribute structure.
func GetAttributes(lst *sxpf.Array) attrs.Attributes {
	pairs := lst.GetValue()
	a := make(attrs.Attributes, len(pairs))
	for _, elem := range pairs {
		l, ok := elem.(*sxpf.Array)
		if !ok {
			continue
		}
		pair := l.GetValue()
		if len(pair) < 2 {
			continue
		}
		key, err := sxpf.GetString(pair, 0)
		if err != nil {
			continue
		}
		val, err := sxpf.GetString(pair, 1)
		if err != nil {
			continue
		}
		a.Set(key, val)
	}
	if len(a) == 0 {
		return nil
	}
	return a
}
