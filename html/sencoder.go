//-----------------------------------------------------------------------------
// Copyright (c) 2022 Detlef Stern
//
// This file is part of zettelstore-client.
//
// Zettelstore client is licensed under the latest version of the EUPL
// (European Union Public License). Please see file LICENSE.txt for your rights
// and obligations under this license.
//-----------------------------------------------------------------------------

package html

import (
	"fmt"
	"io"
	"log"
	"strconv"

	"zettelstore.de/c/attrs"
	"zettelstore.de/c/sexpr"
)

type EncodingFunc func(env *EncEnvironment, args []sexpr.Value)
type encodingMap map[*sexpr.Symbol]EncodingFunc

func (m encodingMap) Clone() encodingMap {
	if l := len(m); l > 0 {
		result := make(encodingMap, l)
		for k, v := range m {
			result[k] = v
		}
		return result
	}
	return nil

}

type EncEnvironment struct {
	err           error
	builtins      encodingMap
	w             io.Writer
	headingOffset int
	unique        string
	footnotes     []sfootnodeInfo
	writeFootnote bool // true iff output should include footnotes and marks
	noLinks       bool // true iff output must not include links
	visibleSpace  bool // true iff space should be "visible" by using EscapeVisible
}
type sfootnodeInfo struct {
	note  []sexpr.Value
	attrs attrs.Attributes
}

func NewEncEnvironment(w io.Writer, headingOffset int) *EncEnvironment {
	return &EncEnvironment{
		builtins:      defaultEncodingFunctions.Clone(),
		w:             w,
		headingOffset: headingOffset,
		footnotes:     nil,
		writeFootnote: true,
	}
}

// SetError marks the environment with an error, if there is not already a marked error.
// The effect is that all future output is not written.
func (env *EncEnvironment) SetError(err error) {
	if env.err == nil {
		env.err = err
	}
}
func (env *EncEnvironment) GetError() error { return env.err }

// ReplaceWriter flushes the previous writer and installs the new one.
func (env *EncEnvironment) ReplaceWriter(w io.Writer) { env.w = w }

func (env *EncEnvironment) SetUnique(s string) {
	if s == "" {
		env.unique = ""
	} else {
		env.unique = ":" + s
	}
}

// IgnoreLinks returns true, if HTML links must not be encoded. This happens if
// the encoded HTML is used in a link itself.
func (env *EncEnvironment) IgnoreLinks() bool { return env.noLinks }

func (env *EncEnvironment) WriteString(s string) {
	if env.err == nil {
		_, env.err = io.WriteString(env.w, s)
	}
}
func (env *EncEnvironment) WriteEscaped(s string) {
	if env.err == nil {
		_, env.err = Escape(env.w, s)
	}
}
func (env *EncEnvironment) WriteEscapedLiteral(s string) {
	if env.err == nil {
		if env.visibleSpace {
			_, env.err = EscapeVisible(env.w, s)
		} else {
			_, env.err = EscapeLiteral(env.w, s)
		}
	}
}

func (env *EncEnvironment) MissingArgs(args []sexpr.Value, minArgs int) bool {
	if len(args) < minArgs {
		env.SetError(fmt.Errorf("required args: %d, but got only: %d", minArgs, len(args)))
		return true
	}
	return false
}
func (env *EncEnvironment) GetSymbol(args []sexpr.Value, idx int) (res *sexpr.Symbol) {
	if env.err != nil {
		return nil
	}
	res, env.err = sexpr.GetSymbol(args, idx)
	return res
}
func (env *EncEnvironment) GetString(args []sexpr.Value, idx int) (res string) {
	if env.err != nil {
		return ""
	}
	res, env.err = sexpr.GetString(args, idx)
	return res
}
func (env *EncEnvironment) GetList(args []sexpr.Value, idx int) (res *sexpr.List) {
	if env.err != nil {
		return nil
	}
	res, env.err = sexpr.GetList(args, idx)
	return res
}

func (env *EncEnvironment) WriteAttributes(a attrs.Attributes) {
	if len(a) == 0 {
		return
	}
	for _, key := range a.Keys() {
		if key == "" || key == attrs.DefaultAttribute {
			continue
		}
		val, found := a.Get(key)
		if !found {
			continue
		}
		env.WriteString(" ")
		env.WriteString(key)
		if val != "" {
			env.WriteString(`="`)
			if env.err == nil {
				_, env.err = AttributeEscape(env.w, val)
			}
			env.WriteString(`"`)
		}
	}
}

func (env *EncEnvironment) WriteStartTag(tag string, a attrs.Attributes) {
	env.WriteString("<")
	env.WriteString(tag)
	env.WriteAttributes(a)
	env.WriteString(">")
}

func (env *EncEnvironment) WriteEndTag(tag string) {
	env.WriteString("</")
	env.WriteString(tag)
	env.WriteString(">")
}

func (env *EncEnvironment) Encode(value sexpr.Value) {
	if env.err != nil {
		return
	}
	switch val := value.(type) {
	case *sexpr.Symbol:
		env.WriteEscaped(val.GetValue())
	case *sexpr.String:
		env.WriteEscaped(val.GetValue())
	case *sexpr.List:
		env.EncodeList(val.GetValue())
	}
}
func (env *EncEnvironment) EncodeList(lst []sexpr.Value) {
	if len(lst) == 0 {
		return
	}
	if sym, ok := lst[0].(*sexpr.Symbol); ok {
		if f, found := env.builtins[sym]; found && f != nil {
			f(env, lst[1:])
			return
		}
		env.SetError(fmt.Errorf("unbound identifier: %q", sym.GetValue()))
		return
	}
	for _, value := range lst {
		env.Encode(value)
	}
}
func (env *EncEnvironment) WriteEndnotes() {
	if len(env.footnotes) == 0 {
		return
	}
	env.WriteString("<ol class=\"zs-endnotes\">")
	for i := 0; len(env.footnotes) > 0; i++ {
		fni := env.footnotes[0]
		env.footnotes = env.footnotes[1:]
		n := strconv.Itoa(i + 1)
		un := env.unique + n
		a := fni.attrs.Clone().AddClass("zs-endnote").Set("value", n)
		if _, found := a.Get("id"); !found {
			a = a.Set("id", "fn:"+un)
		}
		if _, found := a.Get("role"); !found {
			a = a.Set("role", "doc-endnote")
		}
		env.WriteStartTag("li", a)
		env.EncodeList(fni.note)
		env.WriteString(` <a class="zs-endnote-backref" href="#fnref:`)
		env.WriteString(un)
		env.WriteString("\" role=\"doc-backlink\">&#x21a9;&#xfe0e;</a></li>")
	}
	env.footnotes = nil
	env.WriteString("</ol>")
}

var defaultEncodingFunctions = encodingMap{
	sexpr.SymPara: func(env *EncEnvironment, args []sexpr.Value) {
		env.WriteString("<p>")
		env.Encode(sexpr.NewList(args...))
		env.WriteString("</p>")
	},
	sexpr.SymHeading: func(env *EncEnvironment, args []sexpr.Value) {
		if env.MissingArgs(args, 5) {
			return
		}
		nLevel, err := strconv.Atoi(env.GetString(args, 0))
		if err != nil {
			env.SetError(err)
			return
		}
		level := strconv.Itoa(nLevel + env.headingOffset)

		a := sexpr.GetAttributes(env.GetList(args, 1))
		if fragment := env.GetString(args, 3); fragment != "" {
			a = a.Set("id", fragment)
		}

		env.WriteString("<h")
		env.WriteString(level)
		env.WriteAttributes(a)
		env.WriteString(">")
		env.EncodeList(args[4:])
		env.WriteString("</h")
		env.WriteString(level)
		env.WriteString(">")
	},
	sexpr.SymThematic: func(env *EncEnvironment, args []sexpr.Value) {
		env.WriteString("<hr")
		if len(args) > 0 {
			env.WriteAttributes(sexpr.GetAttributes(env.GetList(args, 0)))
		}
		env.WriteString(">")
	},
	sexpr.SymListUnordered: makeListFn("ul"),
	sexpr.SymListOrdered:   makeListFn("ol"),
	sexpr.SymListQuote: func(env *EncEnvironment, args []sexpr.Value) {
		env.WriteString("<blockquote>")
		if len(args) == 1 {
			env.Encode(env.GetList(args, 0))
		} else {
			for i := 0; i < len(args); i++ {
				env.WriteString("<p>")
				env.Encode(env.GetList(args, i))
				env.WriteString("</p>")
			}
		}
		env.WriteString("</blockquote>")
	},
	sexpr.SymDescription: func(env *EncEnvironment, args []sexpr.Value) {
		env.WriteString("<dl>")
		for i := 0; i < len(args); i += 2 {
			env.WriteString("<dt>")
			env.Encode(args[i])
			env.WriteString("</dt>")
			i1 := i + 1
			if len(args) <= i1 {
				continue
			}
			ddlist, ok := args[i1].(*sexpr.List)
			if !ok {
				continue
			}
			for _, dditem := range ddlist.GetValue() {
				env.WriteString("<dd>")
				env.Encode(dditem)
				env.WriteString("</dd>")
			}
		}
		env.WriteString("</dl>")
	},
	sexpr.SymTable: func(env *EncEnvironment, args []sexpr.Value) {
		env.WriteString("<table>")
		if header := env.GetList(args, 0).GetValue(); len(header) > 0 {
			env.WriteString("<thead>")
			env.writeTableRow(header)
			env.WriteString("</thead>")
		}
		if len(args) > 1 {
			env.WriteString("<tbody>")
			for i := 1; i < len(args); i++ {
				env.writeTableRow(env.GetList(args, i).GetValue())
			}
			env.WriteString("</tbody>")
		}
		env.WriteString("</table>")
	},
	sexpr.SymCell:       makeCellFn(""),
	sexpr.SymCellCenter: makeCellFn("center"),
	sexpr.SymCellLeft:   makeCellFn("left"),
	sexpr.SymCellRight:  makeCellFn("right"),
	sexpr.SymRegionBlock: func(env *EncEnvironment, args []sexpr.Value) {
		a := sexpr.GetAttributes(env.GetList(args, 0))
		if val, found := a.Get(""); found {
			a = a.Remove("").AddClass(val)
		}
		env.writeRegion(args, a, "div")
	},
	sexpr.SymRegionQuote: func(env *EncEnvironment, args []sexpr.Value) {
		env.writeRegion(args, nil, "blockquote")
	},
	sexpr.SymRegionVerse: func(env *EncEnvironment, args []sexpr.Value) {
		env.writeRegion(args, nil, "div")
	},
	sexpr.SymVerbatimComment: func(env *EncEnvironment, args []sexpr.Value) {
		if sexpr.GetAttributes(env.GetList(args, 0)).HasDefault() {
			if s := env.GetString(args, 1); s != "" {
				env.WriteString("<!--\n")
				env.WriteEscaped(s)
				env.WriteString("\n-->")
			}
		}
	},
	sexpr.SymVerbatimEval: func(env *EncEnvironment, args []sexpr.Value) {
		a := sexpr.GetAttributes(env.GetList(args, 0)).AddClass("zs-eval")
		env.writeVerbatim(args, a)
	},
	sexpr.SymVerbatimHTML: nil,
	sexpr.SymVerbatimMath: func(env *EncEnvironment, args []sexpr.Value) {
		a := sexpr.GetAttributes(env.GetList(args, 0)).AddClass("zs-math")
		env.writeVerbatim(args, a)
	},
	sexpr.SymVerbatimProg: func(env *EncEnvironment, args []sexpr.Value) {
		a := setProgLang(sexpr.GetAttributes(env.GetList(args, 0)))
		oldVisible := env.visibleSpace
		if a.HasDefault() {
			a = a.RemoveDefault()
			env.visibleSpace = true
		}
		env.writeVerbatim(args, a)
		env.visibleSpace = oldVisible
	},
	sexpr.SymVerbatimZettel: DoNothingFn,
	sexpr.SymBLOB:           nil,
	sexpr.SymTransclude:     nil,
	sexpr.SymText: func(env *EncEnvironment, args []sexpr.Value) {
		if len(args) > 0 {
			env.WriteEscaped(env.GetString(args, 0))
		}
	},
	sexpr.SymSpace: func(env *EncEnvironment, args []sexpr.Value) {
		if len(args) == 0 {
			env.WriteString(" ")
			return
		}
		env.WriteEscaped(env.GetString(args, 0))
	},
	sexpr.SymSoft: func(env *EncEnvironment, _ []sexpr.Value) { env.WriteString(" ") },
	sexpr.SymHard: func(env *EncEnvironment, _ []sexpr.Value) { env.WriteString("<br>") },
	sexpr.SymTag: func(env *EncEnvironment, args []sexpr.Value) {
		if len(args) > 0 {
			env.WriteEscaped(env.GetString(args, 0))
		}
	},
	sexpr.SymLink: func(env *EncEnvironment, args []sexpr.Value) {
		if env.noLinks {
			spanList := sexpr.NewList(sexpr.SymFormatSpan)
			spanList.Append(args...)
			env.Encode(spanList)
			return
		}
		if env.MissingArgs(args, 2) {
			return
		}
		a := sexpr.GetAttributes(env.GetList(args, 0))
		ref := env.GetList(args, 1)
		if ref == nil {
			return
		}
		refPair := ref.GetValue()
		refKind := env.GetSymbol(refPair, 0)
		if refKind == nil {
			return
		}
		refValue := env.GetString(refPair, 1)
		switch {
		case sexpr.SymRefStateExternal.Equal(refKind):
			a = a.Set("href", refValue).AddClass("external")
		case sexpr.SymRefStateZettel.Equal(refKind), sexpr.SymRefStateBased.Equal(refKind), sexpr.SymRefStateHosted.Equal(refKind), sexpr.SymRefStateSelf.Equal(refKind):
			a = a.Set("href", refValue)
		case sexpr.SymRefStateBroken.Equal(refKind):
			a = a.AddClass("broken")
		default:
			log.Println("LINK", sexpr.NewList(args...))
		}
		env.WriteString("<a")
		env.WriteAttributes(a)
		env.WriteString(">")

		if in := args[2:]; len(in) == 0 {
			env.WriteString(refValue)
		} else {
			env.EncodeList(in)
		}
		env.WriteString("</a>")
	},
	sexpr.SymEmbed:     nil,
	sexpr.SymEmbedBLOB: nil,
	sexpr.SymCite:      nil,
	sexpr.SymMark: func(env *EncEnvironment, args []sexpr.Value) {
		if env.noLinks {
			spanList := sexpr.NewList(sexpr.SymFormatSpan)
			spanList.Append(args...)
			env.Encode(spanList)
			return
		}
		if fragment := env.GetString(args, 2); fragment != "" {
			env.WriteString(`<a id="`)
			env.WriteString(env.unique)
			env.WriteString(fragment)
			env.WriteString(`">`)
			env.EncodeList(args[3:])
			env.WriteString("</a>")
		} else {
			env.EncodeList(args[3:])
		}
	},
	sexpr.SymFootnote: func(env *EncEnvironment, args []sexpr.Value) {
		if env.writeFootnote {
			a := sexpr.GetAttributes(env.GetList(args, 0))
			env.footnotes = append(env.footnotes, sfootnodeInfo{args[1:], a})
			n := strconv.Itoa(len(env.footnotes))
			un := env.unique + n
			env.WriteString(`<sup id="fnref:`)
			env.WriteString(un)
			env.WriteString(`"><a class="zs-noteref" href="#fn:`)
			env.WriteString(un)
			env.WriteString(`" role="doc-noteref">`)
			env.WriteString(n)
			env.WriteString(`</a></sup>`)
		}
	},
	sexpr.SymFormatDelete:   makeFormatFn("del"),
	sexpr.SymFormatEmph:     makeFormatFn("em"),
	sexpr.SymFormatInsert:   makeFormatFn("ins"),
	sexpr.SymFormatQuote:    makeFormatFn("q"),
	sexpr.SymFormatSpan:     makeFormatFn("span"),
	sexpr.SymFormatStrong:   makeFormatFn("strong"),
	sexpr.SymFormatSub:      makeFormatFn("sub"),
	sexpr.SymFormatSuper:    makeFormatFn("sup"),
	sexpr.SymLiteralComment: nil,
	sexpr.SymLiteralHTML:    nil,
	sexpr.SymLiteralInput:   func(env *EncEnvironment, args []sexpr.Value) { env.writeLiteral(args, nil, "kbd") },
	sexpr.SymLiteralMath: func(env *EncEnvironment, args []sexpr.Value) {
		a := sexpr.GetAttributes(env.GetList(args, 0)).AddClass("zs-math")
		env.writeLiteral(args, a, "code")
	},
	sexpr.SymLiteralOutput: func(env *EncEnvironment, args []sexpr.Value) { env.writeLiteral(args, nil, "samp") },
	sexpr.SymLiteralProg: func(env *EncEnvironment, args []sexpr.Value) {
		a := setProgLang(sexpr.GetAttributes(env.GetList(args, 0)))
		env.writeLiteral(args, a, "code")
	},
	sexpr.SymLiteralZettel: DoNothingFn,
}

// DoNothingFn is a function that does nothing.
func DoNothingFn(*EncEnvironment, []sexpr.Value) {}

func makeListFn(tag string) EncodingFunc {
	return func(env *EncEnvironment, args []sexpr.Value) {
		env.WriteStartTag(tag, nil)
		for _, items := range args {
			env.WriteStartTag("li", nil)
			env.Encode(items)
			env.WriteEndTag("li")
		}
		env.WriteEndTag(tag)
	}
}

func (env *EncEnvironment) writeTableRow(cells []sexpr.Value) {
	if len(cells) > 0 {
		env.WriteString("<tr>")
		for _, cell := range cells {
			env.Encode(cell)
		}
		env.WriteString("</tr>")
	}
}
func makeCellFn(align string) EncodingFunc {
	return func(env *EncEnvironment, args []sexpr.Value) {
		if align == "" {
			env.WriteString("<td>")
		} else {
			env.WriteString(`<td class="`)
			env.WriteString(align)
			env.WriteString(`">`)
		}
		env.EncodeList(args)
		env.WriteString("</td>")
	}
}

func (env *EncEnvironment) writeRegion(args []sexpr.Value, a attrs.Attributes, tag string) {
	if a == nil {
		a = sexpr.GetAttributes(env.GetList(args, 0))
	}
	env.WriteStartTag(tag, a)
	env.Encode(env.GetList(args, 1))
	if cite := env.GetList(args, 2).GetValue(); len(cite) > 0 {
		env.WriteString("<cite>")
		env.EncodeList(cite)
		env.WriteString("</cite>")
	}
	env.WriteEndTag(tag)
}

func (env *EncEnvironment) writeVerbatim(args []sexpr.Value, a attrs.Attributes) {
	env.WriteString("<pre>")
	env.WriteStartTag("code", a)
	env.WriteEscapedLiteral(env.GetString(args, 1))
	env.WriteString("</code></pre>")
}

func makeFormatFn(tag string) EncodingFunc {
	return func(env *EncEnvironment, args []sexpr.Value) {
		if env.MissingArgs(args, 1) {
			return
		}
		a := sexpr.GetAttributes(env.GetList(args, 0))
		if val, found := a.Get(""); found {
			a = a.Remove("").AddClass(val)
		}
		env.WriteStartTag(tag, a)
		env.EncodeList(args[1:])
		env.WriteEndTag(tag)
	}
}

func (env *EncEnvironment) writeLiteral(args []sexpr.Value, a attrs.Attributes, tag string) {
	if env.MissingArgs(args, 2) {
		return
	}

	if a == nil {
		a = sexpr.GetAttributes(env.GetList(args, 0))
	}
	oldVisible := env.visibleSpace
	if a.HasDefault() {
		env.visibleSpace = true
		a = a.RemoveDefault()
	}
	env.WriteStartTag(tag, a)
	env.visibleSpace = oldVisible
	env.WriteString(env.GetString(args, 1))
	env.WriteEndTag(tag)
}
func setProgLang(a attrs.Attributes) attrs.Attributes {
	if val, found := a.Get(""); found {
		a = a.AddClass("language-" + val).Remove("")
	}
	return a
}
