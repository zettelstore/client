//-----------------------------------------------------------------------------
// Copyright (c) 2023-present Detlef Stern
//
// This file is part of zettelstore-client.
//
// Zettelstore client is licensed under the latest version of the EUPL
// (European Union Public License). Please see file LICENSE.txt for your rights
// and obligations under this license.
//-----------------------------------------------------------------------------

// Package shtml transforms a s-expr encoded zettel AST into a s-expr representation of HTML.
package shtml

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"codeberg.org/t73fde/sxhtml"
	"codeberg.org/t73fde/sxpf"
	"codeberg.org/t73fde/sxpf/eval"
	"zettelstore.de/c/api"
	"zettelstore.de/c/attrs"
	"zettelstore.de/c/sexpr"
	"zettelstore.de/c/text"
)

// Transformer will transform a s-expression that encodes the zettel AST into an s-expression
// that represents HTML.
type Transformer struct {
	sf            sxpf.SymbolFactory
	rebinder      RebindProc
	headingOffset int64
	unique        string
	endnotes      []endnoteInfo
	noLinks       bool // true iff output must not include links
	symAttr       *sxpf.Symbol
	symClass      *sxpf.Symbol
	symMeta       *sxpf.Symbol
	symA          *sxpf.Symbol
	symSpan       *sxpf.Symbol
}

type endnoteInfo struct {
	noteAST *sxpf.List // Endnote as AST
	noteHx  *sxpf.List // Endnote as SxHTML
	attrs   *sxpf.List // attrs a-list
}

// NewTransformer creates a new transformer object.
func NewTransformer(headingOffset int) *Transformer {
	sf := sxpf.MakeMappedFactory()
	return &Transformer{
		sf:            sf,
		rebinder:      nil,
		headingOffset: int64(headingOffset),
		symAttr:       sf.Make(sxhtml.NameSymAttr),
		symClass:      sf.Make("class"),
		symMeta:       sf.Make("meta"),
		symA:          sf.Make("a"),
		symSpan:       sf.Make("span"),
	}
}

// SymbolFactory returns the symbol factory to create HTML symbols.
func (tr *Transformer) SymbolFactory() sxpf.SymbolFactory { return tr.sf }

// Make a new HTML symbol.
func (tr *Transformer) Make(s string) *sxpf.Symbol { return tr.sf.Make(s) }

// RebindProc is a procedure which is called every time before a tranformation takes place.
type RebindProc func(*TransformEnv)

// SetRebinder sets the rebinder procedure.
func (tr *Transformer) SetRebinder(rb RebindProc) { tr.rebinder = rb }

// TransformAttrbute transforms the given attributes into a HTML s-expression.
func (tr *Transformer) TransformAttrbute(a attrs.Attributes) *sxpf.List {
	if len(a) == 0 {
		return sxpf.Nil()
	}
	plist := sxpf.Nil()
	keys := a.Keys()
	for i := len(keys) - 1; i >= 0; i-- {
		key := keys[i]
		if key != attrs.DefaultAttribute {
			plist = plist.Cons(sxpf.Cons(tr.Make(key), sxpf.MakeString(a[key])))
		}
	}
	if plist == nil {
		return sxpf.Nil()
	}
	return plist.Cons(tr.symAttr)
}

// TransformMeta creates a HTML meta s-expression
func (tr *Transformer) TransformMeta(a attrs.Attributes) *sxpf.List {
	return sxpf.Nil().Cons(tr.TransformAttrbute(a)).Cons(tr.symMeta)
}

// Transform an AST s-expression into a list of HTML s-expressions.
func (tr *Transformer) Transform(lst *sxpf.List) (*sxpf.List, error) {
	return tr.TransformInline(lst, false, false)
}

// TransformInline an inlines AST s-expression into a list of HTML s-expressions.
func (tr *Transformer) TransformInline(lst *sxpf.List, noFootnotes, noLinks bool) (*sxpf.List, error) {
	astSF := sxpf.FindSymbolFactory(lst)
	if astSF != nil && astSF == tr.sf {
		panic("Invalid AST SymbolFactory")
	}
	eenv := sxpf.MakeRootEnvironment()
	te := TransformEnv{
		tr:          tr,
		astSF:       astSF,
		eenv:        eenv,
		err:         nil,
		textEnc:     text.NewEncoder(astSF),
		noFootnotes: noFootnotes,
		noLinks:     noLinks,
	}
	te.initialize()
	if rb := tr.rebinder; rb != nil {
		rb(&te)
	}

	val, err := eval.Eval(te.eenv, lst)
	res, ok := val.(*sxpf.List)
	if !ok {
		panic("Result is not a list")
	}
	for i := 0; i < len(tr.endnotes); i++ {
		tr.endnotes[i].noteHx = te.evaluateList(tr.endnotes[i].noteAST) // May extend tr.endnotes
	}
	return res, err
}

// Endnotes returns a SHTML object with all collected endnotes.
func (tr *Transformer) Endnotes() *sxpf.List {
	if len(tr.endnotes) == 0 {
		return nil
	}
	result := sxpf.Nil().Cons(tr.Make("ol"))
	currResult := result.AppendBang(sxpf.Nil().Cons(sxpf.Cons(tr.symClass, sxpf.MakeString("zs-endnotes"))).Cons(tr.symAttr))
	for i, fni := range tr.endnotes {
		noteNum := strconv.Itoa(i + 1)
		noteID := tr.unique + noteNum

		attrs := fni.attrs.Cons(sxpf.Cons(tr.symClass, sxpf.MakeString("zs-endnote"))).
			Cons(sxpf.Cons(tr.Make("value"), sxpf.MakeString(noteNum))).
			Cons(sxpf.Cons(tr.Make("id"), sxpf.MakeString("fn:"+noteID))).
			Cons(sxpf.Cons(tr.Make("role"), sxpf.MakeString("doc-endnote"))).
			Cons(tr.symAttr)

		backref := sxpf.Nil().Cons(sxpf.MakeString("\u21a9\ufe0e")).
			Cons(sxpf.Nil().
				Cons(sxpf.Cons(tr.symClass, sxpf.MakeString("zs-endnote-backref"))).
				Cons(sxpf.Cons(tr.Make("href"), sxpf.MakeString("#fnref:"+noteID))).
				Cons(sxpf.Cons(tr.Make("role"), sxpf.MakeString("doc-backlink"))).
				Cons(tr.symAttr)).
			Cons(tr.symA)

		li := sxpf.Nil().Cons(tr.Make("li"))
		li.AppendBang(attrs).
			ExtendBang(fni.noteHx).
			AppendBang(sxpf.MakeString(" ")).AppendBang(backref)
		currResult = currResult.AppendBang(li)
	}
	return result
}

// TransformEnv is the environment where the actual transformation takes places.
type TransformEnv struct {
	tr          *Transformer
	astSF       sxpf.SymbolFactory
	eenv        sxpf.Environment
	err         error
	textEnc     *text.Encoder
	noFootnotes bool
	noLinks     bool
	symNoEscape *sxpf.Symbol
	symAttr     *sxpf.Symbol
	symMeta     *sxpf.Symbol
	symA        *sxpf.Symbol
	symSpan     *sxpf.Symbol
	symP        *sxpf.Symbol
}

func (te *TransformEnv) initialize() {
	te.symNoEscape = te.Make(sxhtml.NameSymNoEscape)
	te.symAttr = te.tr.symAttr
	te.symMeta = te.tr.symMeta
	te.symA = te.tr.symA
	te.symSpan = te.tr.symSpan
	te.symP = te.Make("p")

	te.bindMetadata()
	te.bindBlocks()
	te.bindInlines()
}

func (te *TransformEnv) bindMetadata() {
	te.bind(sexpr.NameSymMeta, 0, func(args *sxpf.List) sxpf.Object {
		return te.evaluateList(args)
	})
	te.bind(sexpr.NameSymTypeZettelmarkup, 2, func(args *sxpf.List) sxpf.Object {
		a := make(attrs.Attributes, 2).
			Set("name", te.getString(args).String()).
			Set("content", te.textEnc.Encode(te.getList(args.Tail())))
		return te.transformMeta(a)
	})
	metaString := func(args *sxpf.List) sxpf.Object {
		a := make(attrs.Attributes, 2).
			Set("name", te.getString(args).String()).
			Set("content", te.getString(args.Tail()).String())
		return te.transformMeta(a)
	}
	te.bind(sexpr.NameSymTypeCredential, 2, metaString)
	te.bind(sexpr.NameSymTypeEmpty, 2, metaString)
	te.bind(sexpr.NameSymTypeID, 2, metaString)
	te.bind(sexpr.NameSymTypeNumber, 2, metaString)
	te.bind(sexpr.NameSymTypeString, 2, metaString)
	te.bind(sexpr.NameSymTypeTimestamp, 2, metaString)
	te.bind(sexpr.NameSymTypeURL, 2, metaString)
	te.bind(sexpr.NameSymTypeWord, 2, metaString)
	metaSet := func(args *sxpf.List) sxpf.Object {
		var sb strings.Builder
		for elem := te.getList(args.Tail()); elem != nil; elem = elem.Tail() {
			sb.WriteByte(' ')
			sb.WriteString(te.getString(elem).String())
		}
		s := sb.String()
		if len(s) > 0 {
			s = s[1:]
		}
		a := make(attrs.Attributes, 2).
			Set("name", te.getString(args).String()).
			Set("content", s)
		return te.transformMeta(a)
	}
	te.bind(sexpr.NameSymTypeIDSet, 2, metaSet)
	te.bind(sexpr.NameSymTypeTagSet, 2, metaSet)
	te.bind(sexpr.NameSymTypeWordSet, 2, metaSet)
}

func (te *TransformEnv) bindBlocks() {
	te.bind(sexpr.NameSymBlock, 0, func(args *sxpf.List) sxpf.Object {
		return te.evaluateList(args)
	})
	te.bind(sexpr.NameSymPara, 0, func(args *sxpf.List) sxpf.Object {
		return te.evaluateList(args).Cons(te.symP)
	})
	te.bind(sexpr.NameSymHeading, 5, func(args *sxpf.List) sxpf.Object {
		nLevel := te.getInt64(args)
		if nLevel <= 0 {
			te.err = fmt.Errorf("%v is a negative level", nLevel)
			return sxpf.Nil()
		}
		level := strconv.FormatInt(nLevel+te.tr.headingOffset, 10)

		argAttr := args.Tail()
		a := te.getAttributes(argAttr)
		argFragment := argAttr.Tail().Tail()
		if fragment := te.getString(argFragment).String(); fragment != "" {
			a = a.Set("id", te.tr.unique+fragment)
		}

		result := te.evaluateList(argFragment.Tail())
		if len(a) > 0 {
			result = result.Cons(te.transformAttribute(a))
		}
		return result.Cons(te.Make("h" + level))
	})
	te.bind(sexpr.NameSymThematic, 0, func(args *sxpf.List) sxpf.Object {
		result := sxpf.Nil()
		if args != nil {
			if attrList := te.getList(args); attrList != nil {
				result = result.Cons(te.transformAttribute(sexpr.GetAttributes(attrList)))
			}
		}
		return result.Cons(te.Make("hr"))
	})
	te.bind(sexpr.NameSymListOrdered, 0, te.makeListFn("ol"))
	te.bind(sexpr.NameSymListUnordered, 0, te.makeListFn("ul"))
	te.bind(sexpr.NameSymDescription, 0, func(args *sxpf.List) sxpf.Object {
		if args == nil {
			return sxpf.Nil()
		}
		items := sxpf.Nil().Cons(te.Make("dl"))
		curItem := items
		for elem := args; elem != nil; elem = elem.Tail() {
			term, ok := te.evaluate(te.getList(elem)).(*sxpf.List)
			if !ok {
				break
			}
			curItem = curItem.AppendBang(term.Cons(te.Make("dt")))
			elem = elem.Tail()
			if elem == nil {
				break
			}
			ddBlock := te.getList(elem)
			if ddBlock == nil {
				break
			}
			// TODO: assert ddBlock.Head() == symBlock
			for ddlst := ddBlock.Tail(); ddlst != nil; ddlst = ddlst.Tail() {
				dditem := te.getList(ddlst)
				descr, ok2 := te.evaluate(dditem).(*sxpf.List)
				if !ok2 {
					continue
				}
				curItem = curItem.AppendBang(descr.Cons(te.Make("dd")))
			}
		}
		return items
	})

	te.bind(sexpr.NameSymListQuote, 0, func(args *sxpf.List) sxpf.Object {
		if args == nil {
			return sxpf.Nil()
		}
		result := sxpf.Nil().Cons(te.Make("blockquote"))
		currResult := result
		for elem := args; elem != nil; elem = elem.Tail() {
			if quote, ok := elem.Car().(*sxpf.List); ok {
				currResult = currResult.AppendBang(te.evaluateList(quote.Tail()).Cons(te.symP))
			}
		}
		return result
	})

	te.bind(sexpr.NameSymTable, 1, func(args *sxpf.List) sxpf.Object {
		thead := sxpf.Nil()
		if header := te.getList(args); header != nil {
			thead = sxpf.Nil().Cons(te.transformTableRow(header)).Cons(te.Make("thead"))
		}

		tbody := sxpf.Nil()
		if argBody := args.Tail(); argBody != nil {
			tbody = sxpf.Nil().Cons(te.Make("tbody"))
			curBody := tbody
			for row := argBody; row != nil; row = row.Tail() {
				curBody = curBody.AppendBang(te.transformTableRow(te.getList(row)))
			}
		}

		table := sxpf.Nil()
		if tbody != nil {
			table = table.Cons(tbody)
		}
		if thead != nil {
			table = table.Cons(thead)
		}
		if table == nil {
			return sxpf.Nil()
		}
		return table.Cons(te.Make("table"))
	})
	te.bind(sexpr.NameSymCell, 0, te.makeCellFn(""))
	te.bind(sexpr.NameSymCellCenter, 0, te.makeCellFn("center"))
	te.bind(sexpr.NameSymCellLeft, 0, te.makeCellFn("left"))
	te.bind(sexpr.NameSymCellRight, 0, te.makeCellFn("right"))

	te.bind(sexpr.NameSymRegionBlock, 2, te.makeRegionFn(te.Make("div")))
	te.bind(sexpr.NameSymRegionQuote, 2, te.makeRegionFn(te.Make("blockquote")))
	te.bind(sexpr.NameSymRegionVerse, 2, te.makeRegionFn(te.Make("div")))

	te.bind(sexpr.NameSymVerbatimComment, 1, func(args *sxpf.List) sxpf.Object {
		if te.getAttributes(args).HasDefault() {
			if s := te.getString(args.Tail()); s != "" {
				t := sxpf.MakeString(s.String())
				return sxpf.Nil().Cons(t).Cons(te.Make(sxhtml.NameSymBlockComment))
			}
		}
		return nil
	})

	te.bind(sexpr.NameSymVerbatimEval, 2, func(args *sxpf.List) sxpf.Object {
		return te.transformVerbatim(te.getAttributes(args).AddClass("zs-eval"), te.getString(args.Tail()))
	})
	te.bind(sexpr.NameSymVerbatimHTML, 2, te.transformHTML)
	te.bind(sexpr.NameSymVerbatimMath, 2, func(args *sxpf.List) sxpf.Object {
		return te.transformVerbatim(te.getAttributes(args).AddClass("zs-math"), te.getString(args.Tail()))
	})
	te.bind(sexpr.NameSymVerbatimProg, 2, func(args *sxpf.List) sxpf.Object {
		a := setProgLang(te.getAttributes(args))
		content := te.getString(args.Tail())
		if a.HasDefault() {
			content = sxpf.MakeString(visibleReplacer.Replace(content.String()))
		}
		return te.transformVerbatim(a, content)
	})
	te.bind(sexpr.NameSymVerbatimZettel, 0, func(*sxpf.List) sxpf.Object { return sxpf.Nil() })

	te.bind(sexpr.NameSymBLOB, 3, func(args *sxpf.List) sxpf.Object {
		argSyntax := args.Tail()
		return te.transformBLOB(te.getList(args), te.getString(argSyntax), te.getString(argSyntax.Tail()))
	})

	te.bind(sexpr.NameSymTransclude, 2, func(args *sxpf.List) sxpf.Object {
		ref, ok := args.Tail().Car().(*sxpf.List)
		if !ok {
			return sxpf.Nil()
		}
		refKind := ref.Car()
		if sxpf.IsNil(refKind) {
			return sxpf.Nil()
		}
		if refValue := te.getString(ref.Tail()); refValue != "" {
			if te.astSF.Make(sexpr.NameSymRefStateExternal).IsEqual(refKind) {
				a := te.getAttributes(args).Set("src", refValue.String()).AddClass("external")
				return sxpf.Nil().Cons(sxpf.Nil().Cons(te.transformAttribute(a)).Cons(te.Make("img"))).Cons(te.symP)
			}
			return sxpf.MakeList(
				te.Make(sxhtml.NameSymInlineComment),
				sxpf.MakeString("transclude"),
				refKind,
				sxpf.MakeString("->"),
				refValue,
			)
		}
		return args
	})
}

func (te *TransformEnv) makeListFn(tag string) transformFn {
	sym := te.Make(tag)
	return func(args *sxpf.List) sxpf.Object {
		result := sxpf.Nil().Cons(sym)
		last := result
		for elem := args; elem != nil; elem = elem.Tail() {
			item := sxpf.Nil().Cons(te.Make("li"))
			if res, ok := te.evaluate(elem.Car()).(*sxpf.List); ok {
				item.ExtendBang(res)
			}
			last = last.AppendBang(item)
		}
		return result
	}
}
func (te *TransformEnv) transformTableRow(cells *sxpf.List) *sxpf.List {
	row := sxpf.Nil().Cons(te.Make("tr"))
	if cells == nil {
		return sxpf.Nil()
	}
	curRow := row
	for cell := cells.Tail(); cell != nil; cell = cell.Tail() {
		curRow = curRow.AppendBang(te.evaluate(cell.Car()))
	}
	return row
}

func (te *TransformEnv) makeCellFn(align string) transformFn {
	return func(args *sxpf.List) sxpf.Object {
		tdata := te.evaluateList(args)
		if align != "" {
			tdata = tdata.Cons(te.transformAttribute(attrs.Attributes{"class": align}))
		}
		return tdata.Cons(te.Make("td"))
	}
}

func (te *TransformEnv) makeRegionFn(sym *sxpf.Symbol) transformFn {
	return func(args *sxpf.List) sxpf.Object {
		// a := te.getAttributes(args)
		result := sxpf.Nil().Cons(sym)
		currResult := result
		blockArg := args.Tail()
		if region, ok := blockArg.Car().(*sxpf.List); ok {
			currResult = currResult.ExtendBang(te.evaluateList(region.Tail()))
		}
		if citeArg := blockArg.Tail(); citeArg != nil {
			if cite, ok := citeArg.Car().(*sxpf.List); ok && cite.Tail() != nil {
				currResult.AppendBang(te.evaluateList(cite.Tail()).Cons(te.Make("cite")))
			}
		}
		return result
	}
}

func (te *TransformEnv) transformVerbatim(a attrs.Attributes, s sxpf.String) sxpf.Object {
	code := sxpf.Nil().Cons(s)
	if al := te.transformAttribute(a); al != nil {
		code = code.Cons(al)
	}
	code = code.Cons(te.Make("code"))
	return sxpf.Nil().Cons(code).Cons(te.Make("pre"))
}

func (te *TransformEnv) bindInlines() {
	te.bind(sexpr.NameSymInline, 0, func(args *sxpf.List) sxpf.Object {
		return te.evaluateList(args)
	})
	te.bind(sexpr.NameSymText, 1, func(args *sxpf.List) sxpf.Object { return te.getString(args) })
	te.bind(sexpr.NameSymSpace, 0, func(args *sxpf.List) sxpf.Object {
		if args.IsNil() {
			return sxpf.MakeString(" ")
		}
		return te.getString(args)
	})
	te.bind(sexpr.NameSymSoft, 0, func(*sxpf.List) sxpf.Object { return sxpf.MakeString(" ") })
	brSym := te.Make("br")
	te.bind(sexpr.NameSymHard, 0, func(*sxpf.List) sxpf.Object { return sxpf.Nil().Cons(brSym) })
	transformAsSpan := func(args *sxpf.List) sxpf.Object {
		if args.Length() > 2 {
			return te.evaluate(args.Tail().Tail().Cons(args.Car()).Cons(te.Make(sexpr.NameSymFormatSpan)))
		}
		return nil
	}
	te.bind(sexpr.NameSymLinkInvalid, 2, transformAsSpan)
	transformHREF := func(args *sxpf.List) sxpf.Object {
		a := te.getAttributes(args)
		refValue := te.getString(args.Tail())
		return te.transformLink(a.Set("href", refValue.String()), refValue, args.Tail().Tail())
	}
	te.bind(sexpr.NameSymLinkZettel, 2, transformHREF)
	te.bind(sexpr.NameSymLinkSelf, 2, transformHREF)
	te.bind(sexpr.NameSymLinkFound, 2, transformHREF)
	te.bind(sexpr.NameSymLinkBroken, 2, func(args *sxpf.List) sxpf.Object {
		a := te.getAttributes(args)
		refValue := te.getString(args.Tail())
		return te.transformLink(a.AddClass("broken"), refValue, args.Tail().Tail())
	})
	te.bind(sexpr.NameSymLinkHosted, 2, transformHREF)
	te.bind(sexpr.NameSymLinkBased, 2, transformHREF)
	te.bind(sexpr.NameSymLinkQuery, 2, func(args *sxpf.List) sxpf.Object {
		a := te.getAttributes(args)
		refValue := te.getString(args.Tail())
		query := "?" + api.QueryKeyQuery + "=" + url.QueryEscape(refValue.String())
		return te.transformLink(a.Set("href", query), refValue, args.Tail().Tail())
	})
	te.bind(sexpr.NameSymLinkExternal, 2, func(args *sxpf.List) sxpf.Object {
		a := te.getAttributes(args)
		refValue := te.getString(args.Tail())
		return te.transformLink(a.Set("href", refValue.String()).AddClass("external"), refValue, args.Tail().Tail())
	})

	te.bind(sexpr.NameSymEmbed, 3, func(args *sxpf.List) sxpf.Object {
		argRef := args.Tail()
		ref := te.getList(argRef)
		syntax := te.getString(argRef.Tail())
		if syntax == api.ValueSyntaxSVG {
			embedAttr := sxpf.MakeList(
				te.symAttr,
				sxpf.Cons(te.Make("type"), sxpf.MakeString("image/svg+xml")),
				sxpf.Cons(te.Make("src"), sxpf.MakeString("/"+te.getString(ref.Tail()).String()+".svg")),
			)
			return sxpf.MakeList(
				te.Make("figure"),
				sxpf.MakeList(
					te.Make("embed"),
					embedAttr,
				),
			)
		}
		a := te.getAttributes(args)
		a = a.Set("src", string(te.getString(ref.Tail())))
		if d := text.EvaluateInlineString(ref.Tail().Tail().Tail()); d != "" {
			a = a.Set("alt", d)
		}
		return sxpf.MakeList(te.Make("img"), te.transformAttribute(a))
	})
	te.bind(sexpr.NameSymEmbedBLOB, 3, func(args *sxpf.List) sxpf.Object {
		argSyntax := args.Tail()
		a, syntax, data := te.getAttributes(args), te.getString(argSyntax), te.getString(argSyntax.Tail())
		summary, _ := a.Get(api.KeySummary)
		return te.transformBLOB(
			sxpf.MakeList(te.astSF.Make(sexpr.NameSymInline), sxpf.MakeString(summary)),
			syntax,
			data,
		)
	})

	te.bind(sexpr.NameSymCite, 2, func(args *sxpf.List) sxpf.Object {
		result := sxpf.Nil()
		argKey := args.Tail()
		if key := te.getString(argKey); key != "" {
			if text := argKey.Tail(); text != nil {
				result = te.evaluateList(text).Cons(sxpf.MakeString(", "))
			}
			result = result.Cons(key)
		}
		if a := te.getAttributes(args); len(a) > 0 {
			result = result.Cons(te.transformAttribute(a))
		}
		if result == nil {
			return nil
		}
		return result.Cons(te.symSpan)
	})

	te.bind(sexpr.NameSymMark, 3, func(args *sxpf.List) sxpf.Object {
		argFragment := args.Tail().Tail()
		result := te.evaluateList(argFragment.Tail())
		if !te.tr.noLinks {
			if fragment := te.getString(argFragment); fragment != "" {
				a := attrs.Attributes{"id": fragment.String() + te.tr.unique}
				return result.Cons(te.transformAttribute(a)).Cons(te.symA)
			}
		}
		return result.Cons(te.symSpan)
	})

	te.bind(sexpr.NameSymFootnote, 1, func(args *sxpf.List) sxpf.Object {
		if te.noFootnotes {
			return sxpf.Nil()
		}
		attrPlist := sxpf.Nil()
		if a := te.getAttributes(args); len(a) > 0 {
			if attrs := te.transformAttribute(a); attrs != nil {
				attrPlist = attrs.Tail()
			}
		}

		te.tr.endnotes = append(te.tr.endnotes, endnoteInfo{noteAST: args.Tail(), noteHx: nil, attrs: attrPlist})
		noteNum := strconv.Itoa(len(te.tr.endnotes))
		noteID := te.tr.unique + noteNum
		hrefAttr := sxpf.Nil().Cons(sxpf.Cons(te.Make("role"), sxpf.MakeString("doc-noteref"))).
			Cons(sxpf.Cons(te.Make("href"), sxpf.MakeString("#fn:"+noteID))).
			Cons(sxpf.Cons(te.tr.symClass, sxpf.MakeString("zs-noteref"))).
			Cons(te.symAttr)
		href := sxpf.Nil().Cons(sxpf.MakeString(noteNum)).Cons(hrefAttr).Cons(te.symA)
		supAttr := sxpf.Nil().Cons(sxpf.Cons(te.Make("id"), sxpf.MakeString("fnref:"+noteID))).Cons(te.symAttr)
		return sxpf.Nil().Cons(href).Cons(supAttr).Cons(te.Make("sup"))
	})

	te.bind(sexpr.NameSymFormatDelete, 1, te.makeFormatFn("del"))
	te.bind(sexpr.NameSymFormatEmph, 1, te.makeFormatFn("em"))
	te.bind(sexpr.NameSymFormatInsert, 1, te.makeFormatFn("ins"))
	te.bind(sexpr.NameSymFormatQuote, 1, te.transformQuote)
	te.bind(sexpr.NameSymFormatSpan, 1, te.makeFormatFn("span"))
	te.bind(sexpr.NameSymFormatStrong, 1, te.makeFormatFn("strong"))
	te.bind(sexpr.NameSymFormatSub, 1, te.makeFormatFn("sub"))
	te.bind(sexpr.NameSymFormatSuper, 1, te.makeFormatFn("sup"))

	te.bind(sexpr.NameSymLiteralComment, 1, func(args *sxpf.List) sxpf.Object {
		if te.getAttributes(args).HasDefault() {
			if s := te.getString(args.Tail()); s != "" {
				return sxpf.Nil().Cons(s).Cons(te.Make(sxhtml.NameSymInlineComment))
			}
		}
		return nil
	})
	te.bind(sexpr.NameSymLiteralHTML, 2, te.transformHTML)
	kbdSym := te.Make("kbd")
	te.bind(sexpr.NameSymLiteralInput, 2, func(args *sxpf.List) sxpf.Object {
		return te.transformLiteral(args, nil, kbdSym)
	})
	codeSym := te.Make("code")
	te.bind(sexpr.NameSymLiteralMath, 2, func(args *sxpf.List) sxpf.Object {
		a := te.getAttributes(args).AddClass("zs-math")
		return te.transformLiteral(args, a, codeSym)
	})
	sampSym := te.Make("samp")
	te.bind(sexpr.NameSymLiteralOutput, 2, func(args *sxpf.List) sxpf.Object {
		return te.transformLiteral(args, nil, sampSym)
	})
	te.bind(sexpr.NameSymLiteralProg, 2, func(args *sxpf.List) sxpf.Object {
		a := setProgLang(te.getAttributes(args))
		return te.transformLiteral(args, a, codeSym)
	})

	te.bind(sexpr.NameSymLiteralZettel, 0, func(*sxpf.List) sxpf.Object { return sxpf.Nil() })
}

func (te *TransformEnv) makeFormatFn(tag string) transformFn {
	sym := te.Make(tag)
	return func(args *sxpf.List) sxpf.Object {
		a := te.getAttributes(args)
		if val, found := a.Get(""); found {
			a = a.Remove("").AddClass(val)
		}
		res := te.evaluateList(args.Tail())
		if len(a) > 0 {
			res = res.Cons(te.transformAttribute(a))
		}
		return res.Cons(sym)
	}
}
func (te *TransformEnv) transformQuote(args *sxpf.List) sxpf.Object {
	const langAttr = "lang"
	a := te.getAttributes(args)
	langVal, found := a.Get(langAttr)
	if found {
		a = a.Remove(langAttr)
	}
	if val, found2 := a.Get(""); found2 {
		a = a.Remove("").AddClass(val)
	}
	res := te.evaluateList(args.Tail())
	if len(a) > 0 {
		res = res.Cons(te.transformAttribute(a))
	}
	res = res.Cons(te.Make("q"))
	if found {
		res = sxpf.Nil().Cons(res).Cons(te.transformAttribute(attrs.Attributes{}.Set(langAttr, langVal))).Cons(te.Make("span"))
	}
	return res
}

var visibleReplacer = strings.NewReplacer(" ", "\u2423")

func (te *TransformEnv) transformLiteral(args *sxpf.List, a attrs.Attributes, sym *sxpf.Symbol) sxpf.Object {
	if a == nil {
		a = te.getAttributes(args)
	}
	literal := te.getString(args.Tail()).String()
	if a.HasDefault() {
		a = a.RemoveDefault()
		literal = visibleReplacer.Replace(literal)
	}
	res := sxpf.Nil().Cons(sxpf.MakeString(literal))
	if len(a) > 0 {
		res = res.Cons(te.transformAttribute(a))
	}
	return res.Cons(sym)
}

func setProgLang(a attrs.Attributes) attrs.Attributes {
	if val, found := a.Get(""); found {
		a = a.AddClass("language-" + val).Remove("")
	}
	return a
}

func (te *TransformEnv) transformHTML(args *sxpf.List) sxpf.Object {
	if s := te.getString(args.Tail()); s != "" && IsSafe(s.String()) {
		return sxpf.Nil().Cons(s).Cons(te.symNoEscape)
	}
	return nil
}

func (te *TransformEnv) transformBLOB(description *sxpf.List, syntax, data sxpf.String) sxpf.Object {
	if data == "" {
		return sxpf.Nil()
	}
	switch syntax {
	case "":
		return sxpf.Nil()
	case api.ValueSyntaxSVG:
		return sxpf.Nil().Cons(sxpf.Nil().Cons(data).Cons(te.symNoEscape)).Cons(te.symP)
	default:
		imgAttr := sxpf.Nil().Cons(sxpf.Cons(te.Make("src"), sxpf.MakeString("data:image/"+syntax.String()+";base64,"+data.String())))
		if d := text.EvaluateInlineString(description); d != "" {
			imgAttr = imgAttr.Cons(sxpf.Cons(te.Make("alt"), sxpf.MakeString(d)))
		}
		return sxpf.Nil().Cons(sxpf.Nil().Cons(imgAttr.Cons(te.symAttr)).Cons(te.Make("img"))).Cons(te.symP)
	}
}

type transformFn func(*sxpf.List) sxpf.Object

func (te *TransformEnv) bind(name string, minArity int, fn transformFn) {
	te.eenv.Bind(te.astSF.Make(name), eval.MakeSpecial(name, func(_ sxpf.Environment, args *sxpf.List) (sxpf.Object, error) {
		if arity := args.Length(); arity < minArity {
			return sxpf.Nil(), fmt.Errorf("not enough arguments (%d) for form %v (%d)", arity, name, minArity)
		}
		res := fn(args)
		return res, te.err
	}))
}

func (te *TransformEnv) Rebind(name string, fn func(sxpf.Environment, *sxpf.List, sxpf.Callable) sxpf.Object) {
	sym := te.astSF.Make(name)
	obj, found := te.eenv.Resolve(sym)
	if !found {
		panic(sym.String())
	}
	preFn, ok := obj.(sxpf.Callable)
	if !ok {
		panic(sym.String())
	}
	te.eenv.Bind(sym, eval.MakeSpecial(name, func(env sxpf.Environment, args *sxpf.List) (sxpf.Object, error) {
		res := fn(env, args, preFn)
		return res, te.err
	}))
}

func (te *TransformEnv) evaluate(val sxpf.Object) sxpf.Object {
	if te.err == nil {
		res, err := eval.Eval(te.eenv, val)
		if err == nil {
			return res
		}
		te.err = err
	}
	return sxpf.Nil()
}

func (te *TransformEnv) evaluateList(lst *sxpf.List) *sxpf.List {
	if te.err == nil {
		res, _, err := eval.EvalList(te.eenv, lst)
		if err == nil {
			return res
		}
		te.err = err
	}
	return sxpf.Nil()
}

func (te *TransformEnv) Make(name string) *sxpf.Symbol { return te.tr.Make(name) }
func (te *TransformEnv) getString(lst *sxpf.List) sxpf.String {
	if te.err != nil {
		return ""
	}
	val := lst.Car()
	if s, ok := val.(sxpf.String); ok {
		return s
	}
	te.err = fmt.Errorf("%v/%T is not a string", val, val)
	return ""
}
func (te *TransformEnv) getInt64(lst *sxpf.List) int64 {
	if te.err != nil {
		return -1017
	}
	val := lst.Car()
	if num, ok := val.(*sxpf.Number); ok {
		return num.GetInt64()
	}
	te.err = fmt.Errorf("%v/%T is not a number", val, val)
	return -1017
}
func (te *TransformEnv) getList(lst *sxpf.List) *sxpf.List {
	if te.err == nil {
		val := lst.Car()
		if res, ok := val.(*sxpf.List); ok {
			return res
		}
		te.err = fmt.Errorf("%v/%T is not a list", val, val)
	}
	return sxpf.Nil()
}
func (te *TransformEnv) getAttributes(args *sxpf.List) attrs.Attributes {
	return sexpr.GetAttributes(te.getList(args))
}

func (te *TransformEnv) transformLink(a attrs.Attributes, refValue sxpf.String, inline *sxpf.List) sxpf.Object {
	var result *sxpf.List
	if inline.IsNil() {
		result = sxpf.Nil().Cons(refValue)
	} else {
		result = te.evaluateList(inline)
	}
	if te.tr.noLinks {
		return result.Cons(te.symSpan)
	}
	return result.Cons(te.transformAttribute(a)).Cons(te.symA)
}

func (te *TransformEnv) transformAttribute(a attrs.Attributes) *sxpf.List {
	return te.tr.TransformAttrbute(a)
}

func (te *TransformEnv) transformMeta(a attrs.Attributes) *sxpf.List {
	return te.tr.TransformMeta(a)
}

var unsafeSnippets = []string{
	"<script", "</script",
	"<iframe", "</iframe",
}

// IsSafe returns true if the given string does not contain unsafe HTML elements.
func IsSafe(s string) bool {
	lower := strings.ToLower(s)
	for _, snippet := range unsafeSnippets {
		if strings.Contains(lower, snippet) {
			return false
		}
	}
	return true
}
