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
	"bytes"
	"fmt"
	"io"
	"log"
	"strconv"

	"zettelstore.de/c/api"
	"zettelstore.de/c/text"
	"zettelstore.de/c/zjson"
)

// TypeFunc is a function that handles the encoding of a specific ZJSON type.
type TypeFunc func(obj zjson.Object, pos int) (bool, zjson.CloseFunc)
type typeMap map[string]TypeFunc

// Encoder translate a ZJSON object into some HTML text.
type Encoder struct {
	tm            typeMap
	w             io.Writer
	headingOffset int
	unique        string
	footnotes     []footnodeInfo
	writeFootnote bool
	visibleSpace  bool
}
type footnodeInfo struct {
	note  zjson.Array
	attrs zjson.Attributes
}

// NewEncoder creates a new HTML encoder.
func NewEncoder(w io.Writer, headingOffset int) *Encoder {
	enc := &Encoder{
		w:             w,
		headingOffset: headingOffset,
		unique:        "",
		footnotes:     nil,
		writeFootnote: true,
		visibleSpace:  false,
	}
	enc.setupTypeMap()
	return enc
}
func (enc *Encoder) setupTypeMap() {
	enc.tm = typeMap{
		// Block
		zjson.TypeParagraph: func(zjson.Object, int) (bool, zjson.CloseFunc) {
			enc.WriteString("<p>")
			return true, func() { enc.WriteString("</p>") }
		},
		zjson.TypeHeading:         enc.visitHeading,
		zjson.TypeBreakThematic:   func(zjson.Object, int) (bool, zjson.CloseFunc) { enc.WriteString("<hr>"); return false, nil },
		zjson.TypeListBullet:      enc.visitListBullet,
		zjson.TypeListOrdered:     enc.visitListOrdered,
		zjson.TypeDescrList:       enc.visitDescription,
		zjson.TypeListQuotation:   enc.visitQuotation,
		zjson.TypeTable:           enc.visitTable,
		zjson.TypeBlock:           enc.visitBlock,
		zjson.TypePoem:            func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeRegion(obj, "div") },
		zjson.TypeExcerpt:         func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeRegion(obj, "blockquote") },
		zjson.TypeVerbatimCode:    enc.visitVerbatimCode,
		zjson.TypeVerbatimEval:    enc.visitVerbatimEval,
		zjson.TypeVerbatimComment: enc.visitVerbatimComment,
		zjson.TypeVerbatimHTML:    enc.visitHTML,
		zjson.TypeVerbatimMath:    enc.visitVerbatimMath,
		zjson.TypeBLOB:            enc.visitBLOB,

		// Inline
		zjson.TypeText: func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
			enc.WriteString(zjson.GetString(obj, zjson.NameString))
			return false, nil
		},
		zjson.TypeSpace: enc.visitSpace,
		zjson.TypeBreakSoft: func(zjson.Object, int) (bool, zjson.CloseFunc) {
			enc.WriteEOL()
			return false, nil
		},
		zjson.TypeBreakHard: func(zjson.Object, int) (bool, zjson.CloseFunc) {
			enc.WriteString("<br>\n")
			return false, nil
		},
		zjson.TypeTag:            enc.visitTag,
		zjson.TypeLink:           enc.visitLink,
		zjson.TypeEmbed:          enc.visitEmbed,
		zjson.TypeEmbedBLOB:      enc.visitEmbedBLOB,
		zjson.TypeCitation:       enc.visitCite,
		zjson.TypeMark:           enc.visitMark,
		zjson.TypeFootnote:       enc.visitFootnote,
		zjson.TypeFormatDelete:   func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeFormat(obj, "del") },
		zjson.TypeFormatEmph:     func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeFormat(obj, "em") },
		zjson.TypeFormatInsert:   func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeFormat(obj, "ins") },
		zjson.TypeFormatQuote:    func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeFormat(obj, "q") },
		zjson.TypeFormatSpan:     func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeFormat(obj, "span") },
		zjson.TypeFormatStrong:   func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeFormat(obj, "strong") },
		zjson.TypeFormatSub:      func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeFormat(obj, "sub") },
		zjson.TypeFormatSuper:    func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeFormat(obj, "sup") },
		zjson.TypeLiteralCode:    enc.visitLiteralCode,
		zjson.TypeLiteralComment: enc.visitLiteralComment,
		zjson.TypeLiteralInput:   func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeLiteral(obj, "kbd") },
		zjson.TypeLiteralOutput:  func(obj zjson.Object, _ int) (bool, zjson.CloseFunc) { return enc.writeLiteral(obj, "samp") },
		zjson.TypeLiteralHTML:    enc.visitHTML,
		zjson.TypeLiteralMath:    enc.visitLiteralMath,
	}
}

// SetTypeFunc replaces an existing TypeFunc with a new one.
func (enc *Encoder) SetTypeFunc(t string, f TypeFunc) {
	enc.MustGetTypeFunc(t)
	enc.tm[t] = f
}

// ChangeTypeFunc replaces an existing TypeFunc with a new one, but allows
// to use the previous value.
func (enc *Encoder) ChangeTypeFunc(t string, maker func(TypeFunc) TypeFunc) {
	enc.tm[t] = maker(enc.MustGetTypeFunc(t))
}

// GetTypeFunc returns the current TypeFunc for a given value. The additional
// boolean result indicates, whether there exists a TypeFunc.
func (enc *Encoder) GetTypeFunc(t string) (TypeFunc, bool) {
	tf, found := enc.tm[t]
	return tf, found
}

// MustGetTypeFunc returns the TypeFunc for a given type value, but panics if
// there is no TypeFunc.
func (enc *Encoder) MustGetTypeFunc(t string) TypeFunc {
	tf, found := enc.tm[t]
	if !found {
		panic(t)
	}
	return tf
}

func (enc *Encoder) SetUnique(s string) {
	if s == "" {
		enc.unique = ""
	} else {
		enc.unique = ":" + s
	}
}

func (enc *Encoder) TraverseBlock(bn zjson.Array)  { zjson.WalkBlock(enc, bn, 0) }
func (enc *Encoder) TraverseInline(in zjson.Array) { zjson.WalkInline(enc, in, 0) }
func (enc *Encoder) TraverseInlineObjects(val zjson.Value) {
	if a, ok := val.(zjson.Array); ok {
		for i, elem := range a {
			zjson.WalkInlineObject(enc, elem, i)
		}
	}
}
func EncodeInline(baseEnc *Encoder, in zjson.Array) string {
	var buf bytes.Buffer
	enc := Encoder{w: &buf}
	enc.setupTypeMap()
	if baseEnc != nil {
		enc.writeFootnote = baseEnc.writeFootnote
		enc.footnotes = baseEnc.footnotes
	}
	zjson.WalkInline(&enc, in, 0)
	if baseEnc != nil {
		baseEnc.footnotes = enc.footnotes
	}
	return buf.String()
}

func (enc *Encoder) WriteEndnotes() {
	if len(enc.footnotes) == 0 {
		return
	}
	enc.WriteString("\n<ol class=\"zs-endnotes\">\n")
	for i := 0; len(enc.footnotes) > 0; i++ {
		fni := enc.footnotes[0]
		enc.footnotes = enc.footnotes[1:]
		n := strconv.Itoa(i + 1)
		un := enc.unique + n
		a := fni.attrs.Clone().AddClass("zs-endnote").Set("value", n)
		if _, found := a.Get("id"); !found {
			a = a.Set("id", "fn:"+un)
		}
		if _, found := a.Get("role"); !found {
			a = a.Set("role", "doc-endnote")
		}
		enc.WriteString("<li")
		enc.WriteAttributes(a)
		enc.WriteByte('>')
		zjson.WalkInline(enc, fni.note, 0) // May add more footnotes
		enc.WriteString(` <a class="zs-endnote-backref" href="#fnref:`)
		enc.WriteString(un)
		enc.WriteString("\" role=\"doc-backlink\">&#x21a9;&#xfe0e;</a></li>\n")
	}
	enc.footnotes = nil
	enc.WriteString("</ol>\n")
}

func (enc *Encoder) Write(b []byte) (int, error)        { return enc.w.Write(b) }
func (enc *Encoder) WriteString(s string) (int, error)  { return io.WriteString(enc.w, s) }
func (enc *Encoder) WriteByte(b byte) error             { _, err := enc.w.Write([]byte{b}); return err }
func (enc *Encoder) WriteEOL() error                    { return enc.WriteByte('\n') }
func (enc *Encoder) WriteEscaped(s string) (int, error) { return Escape(enc, s) }
func (enc *Encoder) WriteEscapedLiteral(s string) (int, error) {
	if enc.visibleSpace {
		return EscapeVisible(enc, s)
	}
	return EscapeLiteral(enc, s)
}
func (enc *Encoder) WriteAttribute(s string) { AttributeEscape(enc, s) }

func (*Encoder) BlockArray(zjson.Array, int) zjson.CloseFunc  { return nil }
func (*Encoder) InlineArray(zjson.Array, int) zjson.CloseFunc { return nil }
func (enc *Encoder) ItemArray(zjson.Array, int) zjson.CloseFunc {
	enc.WriteString("<li>")
	return func() { enc.WriteString("</li>\n") }
}
func (*Encoder) Unexpected(val zjson.Value, pos int, exp string) {
	log.Printf("?%v %d %T %v\n", exp, pos, val, val)
}

func (enc *Encoder) BlockObject(t string, obj zjson.Object, pos int) (bool, zjson.CloseFunc) {
	if pos > 0 {
		enc.WriteEOL()
	}
	if fun, found := enc.tm[t]; found {
		return fun(obj, pos)
	}
	fmt.Fprintln(enc, obj)
	log.Printf("B%T %v\n", obj, obj)
	return true, nil
}

func (enc *Encoder) visitHeading(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	strLevel := zjson.GetNumber(obj)
	if enc.headingOffset > 0 {
		level, err := strconv.Atoi(strLevel)
		if err != nil {
			return true, nil
		}
		strLevel = strconv.Itoa(level + enc.headingOffset)
	}
	a := zjson.GetAttributes(obj)
	if _, found := a.Get("id"); !found {
		if s := zjson.GetString(obj, zjson.NameString); s != "" {
			a = a.Set("id", s)
		}
	}
	if enc.unique != "" {
		if val, found := a.Get("id"); found {
			a = a.Set("id", enc.unique+val)
		}
	}
	enc.WriteString("<h")
	enc.WriteString(strLevel)
	enc.WriteAttributes(a)
	enc.WriteByte('>')

	return true, func() {
		enc.WriteString("</h")
		enc.WriteString(strLevel)
		enc.WriteByte('>')
	}
}

func (enc *Encoder) visitListBullet(obj zjson.Object, pos int) (bool, zjson.CloseFunc) {
	enc.WriteString("<ul>\n")
	enc.writeListChildren(obj, pos)
	enc.WriteString("</ul>")
	return false, nil
}
func (enc *Encoder) visitListOrdered(obj zjson.Object, pos int) (bool, zjson.CloseFunc) {
	enc.WriteString("<ol>\n")
	enc.writeListChildren(obj, pos)
	enc.WriteString("</ol>")
	return false, nil
}

func (enc *Encoder) writeListChildren(obj zjson.Object, pos int) {
	children := zjson.GetArray(obj, zjson.NameList)
	if children == nil {
		return
	}
	compact := isCompactList(children)
	for i, l := range children {
		ef := enc.ItemArray(children, i)
		if items, ok := l.(zjson.Array); ok {
			enc.writeListItems(items, i, compact)
		} else {
			enc.Unexpected(l, i, "Item block array")
		}
		if ef != nil {
			ef()
		}
	}
}
func isCompactList(children zjson.Array) bool {
	for _, iVal := range children {
		items := zjson.MakeArray(iVal)
		if len(items) < 1 {
			continue
		}
		if len(items) > 1 {
			return false
		}
		// Assert: len(blks) == 1
		obj := zjson.MakeObject(items[0])
		if obj == nil {
			continue
		}
		t := zjson.GetString(obj, zjson.NameType)
		if t != zjson.TypeParagraph {
			return false
		}
	}
	return true
}
func (enc *Encoder) writeListItems(items zjson.Array, pos int, compact bool) {
	if compact && len(items) == 1 {
		if obj := zjson.MakeObject(items[0]); obj != nil {
			if t := zjson.GetString(obj, zjson.NameType); t == zjson.TypeParagraph {
				zjson.WalkInlineChild(enc, obj, pos)
				return
			}
		}
	}
	zjson.WalkBlock(enc, items, pos)
}

func (enc *Encoder) visitDescription(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	descrs := zjson.GetArray(obj, zjson.NameDescrList)
	enc.WriteString("<dl>\n")
	for _, elem := range descrs {
		dObj := zjson.MakeObject(elem)
		if dObj == nil {
			continue
		}
		enc.WriteString("<dt>")
		enc.TraverseInlineObjects(zjson.GetArray(dObj, zjson.NameInline))
		enc.WriteString("</dt>\n")
		descr := zjson.GetArray(dObj, zjson.NameDescription)
		if len(descr) == 0 {
			continue
		}
		for _, ddv := range descr {
			dd := zjson.MakeArray(ddv)
			if len(dd) == 0 {
				continue
			}
			enc.WriteString("<dd>")
			enc.writeDescriptionSlice(dd)
			enc.WriteString("</dd>\n")
		}
	}
	enc.WriteString("</dl>")
	return false, nil
}
func (enc *Encoder) writeDescriptionSlice(dd zjson.Array) {
	if len(dd) == 1 {
		if b := zjson.MakeObject(dd[0]); b != nil {
			if t := zjson.GetString(b, zjson.NameType); t == zjson.TypeParagraph {
				zjson.WalkInlineChild(enc, b, 0)
				return
			}
		}
	}
	zjson.WalkBlock(enc, dd, 0)
}

func (enc *Encoder) visitQuotation(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	enc.WriteString("<blockquote>")
	inPara := false
	for i, item := range zjson.GetArray(obj, zjson.NameList) {
		bl, ok := item.(zjson.Array)
		if !ok {
			enc.Unexpected(item, i, "Quotation array")
			continue
		}
		if p := zjson.GetParagraphInline(bl); p != nil {
			if inPara {
				enc.WriteEOL()
			} else {
				enc.WriteString("<p>")
				inPara = true
			}
			zjson.WalkInline(enc, p, 0)
		} else {
			if inPara {
				enc.WriteString("</p>")
				inPara = false
			}
			zjson.WalkBlock(enc, bl, 0)
		}
	}
	if inPara {
		enc.WriteString("</p>")
	}
	enc.WriteString("</blockquote>")
	return false, nil
}

func (enc *Encoder) visitTable(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	tdata := zjson.GetArray(obj, zjson.NameTable)
	if len(tdata) != 2 {
		return false, nil
	}
	hArray := zjson.MakeArray(tdata[0])
	bArray := zjson.MakeArray(tdata[1])
	enc.WriteString("<table>\n")
	if len(hArray) > 0 {
		enc.WriteString("<thead>\n")
		enc.writeTableRow(hArray, "th")
		enc.WriteString("</thead>\n")
	}
	if len(bArray) > 0 {
		enc.WriteString("<tbody>\n")
		for _, row := range bArray {
			if rArray := zjson.MakeArray(row); rArray != nil {
				enc.writeTableRow(rArray, "td")
			}
		}
		enc.WriteString("</tbody>\n")
	}
	enc.WriteString("</table>")
	return false, nil
}
func (enc *Encoder) writeTableRow(row zjson.Array, tag string) {
	enc.WriteString("<tr>")
	for _, cell := range row {
		if cObj := zjson.MakeObject(cell); cObj != nil {
			enc.WriteByte('<')
			enc.WriteString(tag)
			switch a := zjson.GetString(cObj, zjson.NameString); a {
			case zjson.AlignLeft:
				enc.WriteString(` class="left">`)
			case zjson.AlignCenter:
				enc.WriteString(` class="center">`)
			case zjson.AlignRight:
				enc.WriteString(` class="right">`)
			default:
				enc.WriteByte('>')
			}
			enc.TraverseInlineObjects(zjson.GetArray(cObj, zjson.NameInline))
			enc.WriteString("</")
			enc.WriteString(tag)
			enc.WriteByte('>')
		}
	}
	enc.WriteString("</tr>\n")
}

func (enc *Encoder) visitBlock(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	a := zjson.GetAttributes(obj)
	if val, found := a.Get(""); found {
		zjson.SetAttributes(obj, a.Remove("").AddClass(val))
	}
	return enc.writeRegion(obj, "div")
}

func (enc *Encoder) writeRegion(obj zjson.Object, tag string) (bool, zjson.CloseFunc) {
	enc.WriteByte('<')
	enc.WriteString(tag)
	enc.WriteAttributes(zjson.GetAttributes(obj))
	enc.WriteString(">\n")
	if blocks := zjson.GetArray(obj, zjson.NameBlock); blocks != nil {
		zjson.WalkBlock(enc, blocks, 0)
	}
	if cite := zjson.GetArray(obj, zjson.NameInline); cite != nil {
		enc.WriteString("\n<cite>")
		zjson.WalkInline(enc, cite, 0)
		enc.WriteString("</cite>")
	}
	enc.WriteString("\n</")
	enc.WriteString(tag)
	enc.WriteByte('>')
	return false, nil
}

func (enc *Encoder) visitVerbatimCode(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	a := zjson.GetAttributes(obj)
	saveVisible := enc.visibleSpace
	if a.HasDefault() {
		enc.visibleSpace = true
		a = a.RemoveDefault()
	}
	b, c := enc.writeVerbatim(obj, a)
	enc.visibleSpace = saveVisible
	return b, c
}

func (*Encoder) setProgLang(a zjson.Attributes) zjson.Attributes {
	if val, found := a.Get(""); found {
		a = a.AddClass("language-" + val).Remove("")
	}
	return a
}

func (enc *Encoder) visitVerbatimEval(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	return enc.writeVerbatim(obj, zjson.GetAttributes(obj).AddClass("zs-eval"))
}

func (enc *Encoder) visitVerbatimMath(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	return enc.writeVerbatim(obj, zjson.GetAttributes(obj).AddClass("zs-math"))
}

func (enc *Encoder) writeVerbatim(obj zjson.Object, a zjson.Attributes) (bool, zjson.CloseFunc) {
	enc.WriteString("<pre><code")
	enc.WriteAttributes(a)
	enc.Write([]byte{'>'})
	enc.WriteEscapedLiteral(zjson.GetString(obj, zjson.NameString))
	enc.WriteString("</code></pre>")
	return false, nil
}

func (enc *Encoder) visitVerbatimComment(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	if zjson.GetAttributes(obj).HasDefault() {
		if s := zjson.GetString(obj, zjson.NameString); s != "" {
			enc.WriteString("<!--\n")
			enc.WriteEscaped(s)
			enc.WriteString("\n-->")
		}
	}
	return false, nil
}

func (enc *Encoder) visitBLOB(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	switch s := zjson.GetString(obj, zjson.NameString); s {
	case "":
	case api.ValueSyntaxSVG:
		enc.WriteSVG(obj)
	default:
		enc.WriteDataImage(obj, s, zjson.GetString(obj, zjson.NameString2))
	}
	return false, nil
}
func (enc *Encoder) WriteSVG(obj zjson.Object) {
	if svg := zjson.GetString(obj, zjson.NameString3); svg != "" {
		// TODO: add inline text / title as description
		enc.WriteString("<p>")
		enc.WriteString(svg)
		enc.WriteString("</p>")
	}
}
func (enc *Encoder) WriteDataImage(obj zjson.Object, syntax, title string) {
	if b := zjson.GetString(obj, zjson.NameBinary); b != "" {
		enc.WriteString(`<p><img src="data:image/`)
		enc.WriteString(syntax)
		enc.WriteString(";base64,")
		enc.WriteString(b)
		if title != "" {
			enc.WriteString(`" title="`)
			enc.WriteAttribute(title)
		}
		enc.WriteString(`"></p>`)
	}
}

func (enc *Encoder) InlineObject(t string, obj zjson.Object, pos int) (bool, zjson.CloseFunc) {
	if fun, found := enc.tm[t]; found {
		return fun(obj, pos)
	}
	fmt.Fprintln(enc, obj)
	log.Printf("I%T %v\n", obj, obj)
	return true, nil
}

func (enc *Encoder) visitSpace(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	if s := zjson.GetString(obj, zjson.NameString); s != "" {
		enc.WriteString(s)
	} else {
		enc.Write([]byte{' '})
	}
	return false, nil
}

func (enc *Encoder) visitTag(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	if s := zjson.GetString(obj, zjson.NameString); s != "" {
		enc.WriteByte('#')
		enc.WriteString(s)
	}
	return false, nil
}

func (enc *Encoder) visitLink(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	ref := zjson.GetString(obj, zjson.NameString)
	in := zjson.GetArray(obj, zjson.NameInline)
	if ref == "" {
		return len(in) > 0, nil
	}
	a := zjson.GetAttributes(obj)
	switch q := zjson.GetString(obj, zjson.NameString2); q {
	case zjson.RefStateExternal:
		a = a.Set("href", ref).AddClass("external")
	case zjson.RefStateZettel, zjson.RefStateBased, zjson.RefStateHosted, zjson.RefStateSelf:
		a = a.Set("href", ref)
	case zjson.RefStateBroken:
		a = a.AddClass("broken")
	default:
		log.Println("LINK", q, ref)
	}
	enc.WriteString("<a")
	enc.WriteAttributes(a)
	enc.WriteByte('>')

	children := true
	if len(in) == 0 {
		enc.WriteString(ref)
		children = false
	}
	return children, func() { enc.WriteString("</a>") }
}

func (enc *Encoder) visitEmbed(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	src := zjson.GetString(obj, zjson.NameString)
	if syntax := zjson.GetString(obj, zjson.NameString2); syntax == api.ValueSyntaxSVG {
		enc.WriteString(`<figure><embed type="image/svg+xml" src="`)
		enc.WriteString("/" + src + ".svg")
		enc.WriteString("\" /></figure>\n")
		return false, nil
	}
	zid := api.ZettelID(src)
	if zid.IsValid() {
		src = "/" + src + ".content"
	}
	enc.WriteString(`<img src="`)
	enc.WriteString(src)
	enc.WriteImageTitle(obj)
	return false, nil
}
func (enc *Encoder) WriteImageTitle(obj zjson.Object) {
	if title := zjson.GetArray(obj, zjson.NameInline); len(title) > 0 {
		s := text.EncodeInlineString(title)
		enc.WriteString(`" title="`)
		enc.WriteEscaped(s)
	}
	enc.WriteByte('"')
	enc.WriteAttributes(zjson.GetAttributes(obj))
	enc.WriteByte('>')
}

func (enc *Encoder) visitEmbedBLOB(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	switch s := zjson.GetString(obj, zjson.NameString); s {
	case "":
	case api.ValueSyntaxSVG:
		enc.WriteSVG(obj)
	default:
		enc.WriteDataImage(obj, s, text.EncodeInlineString(zjson.GetArray(obj, zjson.NameInline)))
	}
	return false, nil
}

func (enc *Encoder) visitCite(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	if s := zjson.GetString(obj, zjson.NameString); s != "" {
		enc.WriteString(s)
		if zjson.GetArray(obj, zjson.NameInline) != nil {
			enc.WriteString(", ")
		}
	}
	return true, nil
}

func (enc *Encoder) visitMark(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	if q := zjson.GetString(obj, zjson.NameString2); q != "" {
		enc.WriteString(`<a id="`)
		enc.WriteString(enc.unique)
		enc.WriteString(q)
		enc.WriteString(`">`)
		return true, func() { enc.WriteString("</a>") }
	}
	return true, nil
}

func (enc *Encoder) visitFootnote(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	if enc.writeFootnote {
		if fn := zjson.GetArray(obj, zjson.NameInline); fn != nil {
			enc.footnotes = append(enc.footnotes, footnodeInfo{fn, zjson.GetAttributes(obj)})
			n := strconv.Itoa(len(enc.footnotes))
			un := enc.unique + n
			enc.WriteString(`<sup id="fnref:`)
			enc.WriteString(un)
			enc.WriteString(`"><a class="zs-noteref" href="#fn:`)
			enc.WriteString(un)
			enc.WriteString(`" role="doc-noteref">`)
			enc.WriteString(n)
			enc.WriteString(`</a></sup>`)
		}
	}
	return false, nil
}

func (enc *Encoder) writeFormat(obj zjson.Object, tag string) (bool, zjson.CloseFunc) {
	enc.WriteByte('<')
	enc.WriteString(tag)
	a := zjson.GetAttributes(obj)
	if val, found := a.Get(""); found {
		a = a.Remove("").AddClass(val)
	}
	enc.WriteAttributes(a)
	enc.WriteByte('>')
	return true, func() {
		enc.WriteString("</")
		enc.WriteString(tag)
		enc.WriteByte('>')
	}
}

func (enc *Encoder) visitLiteralCode(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	zjson.SetAttributes(obj, enc.setProgLang(zjson.GetAttributes(obj)))
	return enc.writeLiteral(obj, "code")
}

func (enc *Encoder) visitLiteralMath(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	zjson.SetAttributes(obj, zjson.GetAttributes(obj).AddClass("zs-math"))
	return enc.writeLiteral(obj, "code")
}

func (enc *Encoder) writeLiteral(obj zjson.Object, tag string) (bool, zjson.CloseFunc) {
	if s := zjson.GetString(obj, zjson.NameString); s != "" {
		a := zjson.GetAttributes(obj)
		oldVisible := enc.visibleSpace
		if a.HasDefault() {
			enc.visibleSpace = true
			a = a.RemoveDefault()
		}
		enc.WriteByte('<')
		enc.WriteString(tag)
		enc.WriteAttributes(a)
		enc.WriteByte('>')
		enc.WriteEscapedLiteral(s)
		enc.WriteString("</")
		enc.WriteString(tag)
		enc.WriteByte('>')
		enc.visibleSpace = oldVisible
	}
	return false, nil
}

func (enc *Encoder) visitLiteralComment(obj zjson.Object, pos int) (bool, zjson.CloseFunc) {
	if zjson.GetAttributes(obj).HasDefault() {
		if s := zjson.GetString(obj, zjson.NameString); s != "" {
			if pos > 0 {
				enc.WriteString(" <!-- ")
			} else {
				enc.WriteString("<!-- ")
			}
			enc.WriteEscaped(s)
			enc.WriteString(" -->")
		}
	}
	return false, nil
}

func (enc *Encoder) visitHTML(obj zjson.Object, _ int) (bool, zjson.CloseFunc) {
	if s := zjson.GetString(obj, zjson.NameString); s != "" && IsSafe(s) {
		enc.WriteString(s)
	}
	return false, nil
}

func (enc *Encoder) WriteAttributes(a zjson.Attributes) {
	if len(a) == 0 {
		return
	}
	for _, key := range a.Keys() {
		if key == "" || key == "-" {
			continue
		}
		val, found := a.Get(key)
		if !found {
			continue
		}
		enc.WriteByte(' ')
		enc.WriteString(key)
		enc.WriteString(`="`)
		enc.WriteAttribute(val)
		enc.WriteByte('"')
	}
}
