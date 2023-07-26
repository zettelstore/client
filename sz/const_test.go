//-----------------------------------------------------------------------------
// Copyright (c) 2023-present Detlef Stern
//
// This file is part of zettelstore-client.
//
// Zettelstore client is licensed under the latest version of the EUPL
// (European Union Public License). Please see file LICENSE.txt for your rights
// and obligations under this license.
//-----------------------------------------------------------------------------

package sz_test

import (
	"testing"

	"zettelstore.de/c/sz"
	"zettelstore.de/sx.fossil/sxpf"
)

func BenchmarkInitializeZettelSymbols(b *testing.B) {
	sf := sxpf.MakeMappedFactory()
	for i := 0; i < b.N; i++ {
		var zs sz.ZettelSymbols
		zs.InitializeZettelSymbols(sf)
	}
}
