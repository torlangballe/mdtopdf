/*
 * Markdown to PDF Converter
 * Available at http://github.com/mandolyte/mdtopdf
 *
 * Copyright © 2018 Cecil New <cecil.new@gmail.com>.
 * Distributed under the MIT License.
 * See README.md for details.
 *
 * Dependencies
 * This package depends on two other packages:
 *
 * Blackfriday Markdown Processor
 *   Available at http://github.com/russross/blackfriday
 *
 * gofpdf - a PDF document generator with high level support for
 *   text, drawing and images.
 *   Available at https://github.com/torlangballe/gofpdfv2
 */

package mdtopdf

import (
	"fmt"
	"path"
	"strings"

	bf "github.com/torlangballe/blackfridayV2"
	gofpdf "github.com/torlangballe/gofpdfv2"
	"github.com/torlangballe/zutil/zfile"
	"github.com/torlangballe/zutil/zhttp"
	"github.com/torlangballe/zutil/zlog"
)

func (r *PdfRenderer) processText(node *bf.Node) {
	if r.IsInImage {
		return
	}

	r.IsInText = false
	currentStyle := r.cs.peek().textStyle
	// zlog.Info("processText1", string(node.Literal), r.Pdf.Err())
	r.setStyler(currentStyle)
	s := string(node.Literal)
	s = strings.Replace(s, "\n", " ", -1)
	if r.TrimNext {
		s = strings.TrimLeft(s, " ")
		r.TrimNext = false
	}
	r.tracer("Text", s)
	if r.cs.peek().containerType == bf.Link {
		sdest := r.cs.peek().destination
		if !zhttp.HasURLScheme(sdest) {
			id, got := r.anchorLinks[sdest]
			if !got {
				id = r.Pdf.AddLink()
				r.anchorLinks[sdest] = id
			}
			r.writeAnchorLink(currentStyle, s, id)
		} else {
			r.writeLink(currentStyle, s, sdest)
		}
	} else if r.cs.peek().containerType == bf.Heading {
		r.IsInText = true
		//r.cr() // add space before heading
		hid := "#" + node.Parent.HeadingData.HeadingID
		id, got := r.anchorLinks[hid]
		// zlog.Info("Output Anchor?", id, got, hid)
		if !got {
			id = r.Pdf.AddLink()
			r.anchorLinks[hid] = id
		}
		// fmt.Printf("AddHeading: %p %s %s %d %+v\n", node, s, node.Parent.HeadingData.HeadingID, r.anchorLinks[hid], anchorLinks)
		r.Pdf.SetLink(id, -1, r.Pdf.PageNo())
		r.write(currentStyle, s)
	} else if r.cs.peek().containerType == bf.TableCell {
		r.IsInText = true
		if r.cs.peek().isHeader {
			r.setStyler(currentStyle)
			// get the string width of header value
			hw := r.Pdf.GetStringWidth(s) + (2 * r.em)
			// now append it
			cellwidths = append(cellwidths, hw)
			// now write it...
			h, _ := r.Pdf.GetFontSize()
			h += currentStyle.Spacing
			r.tracer("... table header cell",
				fmt.Sprintf("Width=%v, height=%v", hw, h))

			r.Pdf.CellFormat(hw, h, s, "1", 0, "C", true, 0, "")
		} else {
			r.setStyler(currentStyle)
			hw := cellwidths[curdatacell]
			h := currentStyle.Size + currentStyle.Spacing
			r.tracer("... table body cell",
				fmt.Sprintf("Width=%v, height=%v", hw, h))
			r.Pdf.CellFormat(hw, h, s, "LR", 0, "", fill, 0, "")
		}
	} else {
		r.IsInText = (len(s) != 0)
		// zlog.Info("ProcessText4:", len(s) != 0, currentStyle)
		r.write(currentStyle, s)
	}
	if r.ParagraphUnprocessed && r.cs.peek().listkind != notlist && s != "" {
		if r.StrongOn {
			// fmt.Println("First Text in list paragraph:", r.ParagraphUnprocessed, r.cs.peek().listkind, s)
			r.TrimNext = true
			r.cr() // start on next line!
		}
		r.ParagraphUnprocessed = false
	}
}

func (r *PdfRenderer) processCodeblock(node *bf.Node) {
	// r.tracer("Codeblock", fmt.Sprintf("%v", node.CodeBlockData))
	r.setStyler(r.Backtick)
	r.cr() // start on next line!
	r.multiCell(r.Backtick, string(node.Literal))
	/*
		lines := strings.Split(strings.TrimSpace(string(node.Literal)), "\n")
		for n := range lines {
			r.Pdf.CellFormat(0, r.Backtick.Size,
				lines[n], "", 1, "LT", true, 0, "")
		}
	*/
}

func (r *PdfRenderer) processList(node *bf.Node, entering bool) {
	kind := unordered
	if node.ListFlags&bf.ListTypeOrdered != 0 {
		kind = ordered
	}
	if node.ListFlags&bf.ListTypeDefinition != 0 {
		kind = definition
	}
	r.setStyler(r.Normal)
	if entering {
		r.tracer(fmt.Sprintf("%v List (entering)", kind),
			fmt.Sprintf("%v", node.ListData))
		r.Pdf.SetLeftMargin(r.cs.peek().leftMargin + r.IndentValue)
		r.tracer("... List Left Margin",
			fmt.Sprintf("set to %v", r.cs.peek().leftMargin+r.IndentValue))
		x := &containerState{containerType: bf.List,
			textStyle: r.Normal, itemNumber: 0,
			listkind:   kind,
			leftMargin: r.cs.peek().leftMargin + r.IndentValue}
		//		r.cr()
		// before pushing check to see if this is a sublist
		// if so, then output a newline
		/*
			if r.cs.peek().containerType == bf.Item {
				r.cr()
			}
		*/
		r.cs.push(x)
	} else {
		r.tracer(fmt.Sprintf("%v List (leaving)", kind),
			fmt.Sprintf("%v", node.ListData))
		r.Pdf.SetLeftMargin(r.cs.peek().leftMargin - r.IndentValue)
		r.tracer("... Reset List Left Margin",
			fmt.Sprintf("re-set to %v", r.cs.peek().leftMargin-r.IndentValue))
		r.cs.pop()
		if len(r.cs.stack) < 2 {
			r.cr()
		}
	}
}

func (r *PdfRenderer) processItem(node *bf.Node, entering bool) {
	if entering {
		r.tracer(fmt.Sprintf("%v Item (entering) #%v",
			r.cs.peek().listkind, r.cs.peek().itemNumber+1),
			fmt.Sprintf("%v", node.ListData))
		// fmt.Println("processItem", r.cs.peek().listkind, entering, string(node.Literal))
		r.cr() // newline before getting started
		x := &containerState{containerType: bf.Item,
			textStyle: r.Normal, itemNumber: r.cs.peek().itemNumber + 1,
			listkind:       r.cs.peek().listkind,
			firstParagraph: true,
			leftMargin:     r.cs.peek().leftMargin}
		// add bullet or itemnumber; then set left margin for the
		// text/paragraphs in the item
		r.cs.push(x)
		if r.cs.peek().listkind == unordered {
			r.Pdf.CellFormat(3*r.em, r.Normal.Size+r.Normal.Spacing,
				"*",
				"", 0, "RB", false, 0, "")
			// fmt.Println("Output bullet")
		} else if r.cs.peek().listkind == ordered {
			r.Pdf.CellFormat(3*r.em, r.Normal.Size+r.Normal.Spacing,
				fmt.Sprintf("%v.", r.cs.peek().itemNumber),
				"", 0, "RB", false, 0, "")
		}
		// with the bullet done, now set the left margin for the text
		r.Pdf.SetLeftMargin(r.cs.peek().leftMargin + (4 * r.em))
		// set the cursor to this point
		r.Pdf.SetX(r.cs.peek().leftMargin + (4 * r.em))
	} else {
		r.tracer(fmt.Sprintf("%v Item (leaving)",
			r.cs.peek().listkind),
			fmt.Sprintf("%v", node.ListData))
		// before we output the new line, reset left margin
		r.Pdf.SetLeftMargin(r.cs.peek().leftMargin)
		r.cr()
		r.cs.parent().itemNumber++
		r.cs.pop()
	}
}

func (r *PdfRenderer) processEmph(node *bf.Node, entering bool) {
	if entering {
		r.tracer("Emph (entering)", "")
		if !strings.Contains(r.cs.peek().textStyle.Style, "i") {
			r.cs.peek().textStyle.Style += "i"
		}
		// fmt.Println("processEmph", entering, r.cs.peek().textStyle.Style)
	} else {
		r.tracer("Emph (leaving)", "")
		r.cs.peek().textStyle.Style = strings.Replace(
			r.cs.peek().textStyle.Style, "i", "", -1)
	}
}

func (r *PdfRenderer) processStrong(node *bf.Node, entering bool) {
	r.StrongOn = entering
	if entering {
		r.tracer("Strong (entering)", "")
		s := r.cs.peek().textStyle.Style
		if !strings.Contains(s, "b") {
			s += "b"
		}
		r.cs.peek().textStyle.Style = s
	} else {
		r.tracer("Strong (leaving)", "")
		r.cs.peek().textStyle.Style = strings.Replace(
			r.cs.peek().textStyle.Style, "b", "", -1)
	}
}

func (r *PdfRenderer) processLink(node *bf.Node, entering bool) {
	if entering {
		// fmt.Println("PdfRenderer processLink:", node.HeadingData.Level, string(node.LinkData.Destination))
		styler := r.Link
		if r.CurrentHeaderStyler != nil {
			styler.Size = r.CurrentHeaderStyler.Size
		}
		x := &containerState{containerType: bf.Link,
			textStyle: styler, listkind: notlist,
			leftMargin:  r.cs.peek().leftMargin,
			destination: string(node.LinkData.Destination)}
		r.cs.push(x)
		r.tracer("Link (entering)",
			fmt.Sprintf("Destination[%v] Title[%v]",
				string(node.LinkData.Destination),
				string(node.LinkData.Title)))
	} else {
		r.tracer("Link (leaving)", "")
		r.cs.pop()
	}
}

func (r *PdfRenderer) processImage(node *bf.Node, entering bool) {
	// while this has entering and leaving states, it doesn't appear
	// to be useful except for other markup languages to close the tag
	r.IsInImage = entering
	if entering {
		r.tracer("Image (entering)",
			fmt.Sprintf("Destination[%v] Title[%v]",
				string(node.LinkData.Destination),
				string(node.LinkData.Title)))
		// following changes suggested by @sirnewton01, issue #6
		// does file exist?
		var imgPath = r.LocalFilePathPrefix + string(node.LinkData.Destination)
		var multiplyDPI float64
		if zfile.NotExists(imgPath) {
			multiplyDPI = 3
			imgPath = r.LocalImagePathAlternativePrefix + string(node.LinkData.Destination)
		}
		imgPath = path.Clean(imgPath)
		// fmt.Println("PdfRenderer processImage:", imgPath, r.IsInText, r.Pdf.FileSystem != nil)
		canOpen := zfile.CanOpenInFS(r.Pdf.FileSystem, imgPath)
		if canOpen {
			flow := true
			r.Pdf.ImageOptions(string(imgPath),
				-1, 0, -1, -1, flow,
				gofpdf.ImageOptions{ImageType: "", ReadDpi: true, MultiplyDPI: multiplyDPI, IsInline: r.IsInText}, 0, "")
		} else {
			zlog.Error(nil, "Can't open image;", imgPath)
			r.tracer("Image (file error) can't open", imgPath)
		}
	} else {
		r.tracer("Image (leaving)", "")
	}
}

func (r *PdfRenderer) processCode(node *bf.Node) {
	r.tracer("Code", "")
	r.setStyler(r.Backtick)
	r.write(r.Backtick, string(node.Literal))
}

func (r *PdfRenderer) processParagraph(node *bf.Node, entering bool) {
	r.setStyler(r.Normal)
	r.ParagraphUnprocessed = true
	r.TrimNext = false

	if entering {
		r.tracer("Paragraph (entering)", "")
		lm, tm, rm, bm := r.Pdf.GetMargins()
		r.tracer("... Margins (left, top, right, bottom:",
			fmt.Sprintf("%v %v %v %v", lm, tm, rm, bm))
		if r.cs.peek().containerType == bf.Item {
			t := r.cs.peek().listkind
			if t == unordered || t == ordered || t == definition {
				if r.cs.peek().firstParagraph {
					r.tracer("First Para within a list", "breaking")
				} else {
					r.tracer("Not First Para within a list", "indent etc.")
					r.cr()
				}
			}
			return
		}
		r.cr()
		//r.cr()
	} else {
		r.tracer("Paragraph (leaving)", "")
		lm, tm, rm, bm := r.Pdf.GetMargins()
		r.tracer("... Margins (left, top, right, bottom:",
			fmt.Sprintf("%v %v %v %v", lm, tm, rm, bm))
		if r.cs.peek().containerType == bf.Item {
			t := r.cs.peek().listkind
			if t == unordered || t == ordered || t == definition {
				if r.cs.peek().firstParagraph {
					r.cs.peek().firstParagraph = false
				} else {
					r.tracer("Not First Para within a list", "")
					r.cr()
				}
			}
			return
		}
		//r.cr()
		r.cr()
	}
}

func (r *PdfRenderer) processBlockQuote(node *bf.Node, entering bool) {
	if entering {
		r.tracer("BlockQuote (entering)", "")
		curleftmargin, _, _, _ := r.Pdf.GetMargins()
		x := &containerState{containerType: bf.BlockQuote,
			textStyle: r.Blockquote, listkind: notlist,
			leftMargin: curleftmargin + r.IndentValue}
		r.cs.push(x)
		r.Pdf.SetLeftMargin(curleftmargin + r.IndentValue)
	} else {
		r.tracer("BlockQuote (leaving)", "")
		curleftmargin, _, _, _ := r.Pdf.GetMargins()
		r.Pdf.SetLeftMargin(curleftmargin - r.IndentValue)
		r.cs.pop()
		r.cr()
	}
}

func (r *PdfRenderer) processHeading(node *bf.Node, entering bool) {
	if entering {
		r.cr()
		//r.inHeading = true
		switch node.HeadingData.Level {
		case 1:
			r.tracer("Heading (1, entering)", fmt.Sprintf("%v", node.HeadingData))
			r.CurrentHeaderStyler = &r.H1
			x := &containerState{containerType: bf.Heading,
				textStyle: r.H1, listkind: notlist,
				leftMargin: r.cs.peek().leftMargin}
			r.cs.push(x)
		case 2:
			r.tracer("Heading (2, entering)", fmt.Sprintf("%v", node.HeadingData))
			r.CurrentHeaderStyler = &r.H2
			x := &containerState{containerType: bf.Heading,
				textStyle: r.H2, listkind: notlist,
				leftMargin: r.cs.peek().leftMargin}
			r.cs.push(x)
		case 3:
			r.tracer("Heading (3, entering)", fmt.Sprintf("%v", node.HeadingData))
			r.CurrentHeaderStyler = &r.H3
			x := &containerState{containerType: bf.Heading,
				textStyle: r.H3, listkind: notlist,
				leftMargin: r.cs.peek().leftMargin}
			r.cs.push(x)
		case 4:
			r.tracer("Heading (4, entering)", fmt.Sprintf("%v", node.HeadingData))
			r.CurrentHeaderStyler = &r.H4
			x := &containerState{containerType: bf.Heading,
				textStyle: r.H4, listkind: notlist,
				leftMargin: r.cs.peek().leftMargin}
			r.cs.push(x)
		case 5:
			r.tracer("Heading (5, entering)", fmt.Sprintf("%v", node.HeadingData))
			r.CurrentHeaderStyler = &r.H5
			x := &containerState{containerType: bf.Heading,
				textStyle: r.H5, listkind: notlist,
				leftMargin: r.cs.peek().leftMargin}
			r.cs.push(x)
		case 6:
			r.tracer("Heading (6, entering)", fmt.Sprintf("%v", node.HeadingData))
			r.CurrentHeaderStyler = &r.H6
			x := &containerState{containerType: bf.Heading,
				textStyle: r.H6, listkind: notlist,
				leftMargin: r.cs.peek().leftMargin}
			r.cs.push(x)
		}
	} else {
		r.CurrentHeaderStyler = nil
		r.tracer("Heading (leaving)", "")
		r.cr()
		r.cs.pop()
	}
}

func (r *PdfRenderer) processHorizontalRule(node *bf.Node) {
	r.tracer("HorizontalRule", "")
	// do a newline
	r.cr()
	// get the current x and y (assume left margin in ok)
	x, y := r.Pdf.GetXY()
	// get the page margins
	lm, _, _, _ := r.Pdf.GetMargins()
	// get the page size
	w, _ := r.Pdf.GetPageSize()
	// now compute the x value of the right side of page
	newx := w - lm
	r.tracer("... From X,Y", fmt.Sprintf("%v,%v", x, y))
	r.Pdf.MoveTo(x, y)
	r.tracer("...   To X,Y", fmt.Sprintf("%v,%v", newx, y))
	r.Pdf.LineTo(newx, y)
	r.Pdf.SetLineWidth(3)
	r.Pdf.SetFillColor(200, 200, 200)
	r.Pdf.DrawPath("F")
	// another newline
	r.cr()
}

func (r *PdfRenderer) processHTMLBlock(node *bf.Node) {
	r.tracer("HTMLBlock", string(node.Literal))
	r.cr()
	r.setStyler(r.Backtick)
	r.Pdf.CellFormat(0, r.Backtick.Size,
		string(node.Literal), "", 1, "LT", true, 0, "")
	r.cr()
}

func (r *PdfRenderer) processTable(node *bf.Node, entering bool) {
	if entering {
		r.tracer("Table (entering)", "")
		x := &containerState{containerType: bf.Table,
			textStyle: r.THeader, listkind: notlist,
			leftMargin: r.cs.peek().leftMargin}
		r.cr()
		r.cs.push(x)
		fill = false
	} else {
		wSum := 0.0
		for _, w := range cellwidths {
			wSum += w
		}
		r.Pdf.CellFormat(wSum, 0, "", "T", 0, "", false, 0, "")

		r.cs.pop()
		r.tracer("Table (leaving)", "")
		r.cr()
	}
}

func (r *PdfRenderer) processTableHead(node *bf.Node, entering bool) {
	if entering {
		r.tracer("TableHead (entering)", "")
		x := &containerState{containerType: bf.TableHead,
			textStyle: r.THeader, listkind: notlist,
			leftMargin: r.cs.peek().leftMargin}
		r.cs.push(x)
		cellwidths = make([]float64, 0)
	} else {
		r.cs.pop()
		r.tracer("TableHead (leaving)", "")
	}
}

func (r *PdfRenderer) processTableBody(node *bf.Node, entering bool) {
	if entering {
		r.tracer("TableBody (entering)", "")
		x := &containerState{containerType: bf.TableBody,
			textStyle: r.TBody, listkind: notlist,
			leftMargin: r.cs.peek().leftMargin}
		r.cs.push(x)
	} else {
		r.cs.pop()
		r.tracer("TableBody (leaving)", "")
		r.Pdf.Ln(-1)
	}
}

func (r *PdfRenderer) processTableRow(node *bf.Node, entering bool) {
	if entering {
		r.tracer("TableRow (entering)", "")
		x := &containerState{containerType: bf.TableRow,
			textStyle: r.TBody, listkind: notlist,
			leftMargin: r.cs.peek().leftMargin}
		if r.cs.peek().isHeader {
			x.textStyle = r.THeader
		}
		r.Pdf.Ln(-1)

		// initialize cell widths slice; only one table at a time!
		curdatacell = 0
		r.cs.push(x)
	} else {
		r.cs.pop()
		r.tracer("TableRow (leaving)", "")
		fill = !fill
	}
}

func (r *PdfRenderer) processTableCell(node *bf.Node, entering bool) {
	if entering {
		r.tracer("TableCell (entering)", "")
		x := &containerState{containerType: bf.TableCell,
			textStyle: r.Normal, listkind: notlist,
			leftMargin: r.cs.peek().leftMargin}
		if node.TableCellData.IsHeader {
			r.Pdf.SetDrawColor(128, 0, 0)
			r.Pdf.SetLineWidth(.3)
			x.isHeader = true
			x.textStyle = r.THeader
			r.setStyler(r.THeader)
		} else {
			x.textStyle = r.TBody
			r.setStyler(r.TBody)
			x.isHeader = false
		}
		r.cs.push(x)
	} else {
		r.cs.pop()
		r.tracer("TableCell (leaving)", "")
		curdatacell++
	}
}
