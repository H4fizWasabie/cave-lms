package app

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io/fs"
	"strings"
	"sync"

	"github.com/go-pdf/fpdf"
)

// The built-in Certificate artwork: the client's design (sample PCC-2026-00509) drawn at print quality on A4 portrait,
// in English only. drawArtwork draws the constant layer; certificatePDF fills in the details on top, at the same places
// as on an uploaded background. The web preview, template certificate-preview in layout.html, draws the same artwork in
// SVG in the same millimetre coordinates: change the two together.

// Colours, as the stylesheet's tokens: charcoal, ink 2, logo gold, deep gold, light gold (60% logo gold on surface)
// and the text gold (60% logo gold, 40% deep gold).
var (
	artInk       = [3]int{31, 33, 36}
	artInk2      = [3]int{57, 60, 65}
	artGold      = [3]int{210, 166, 73}
	artGoldDeep  = [3]int{134, 97, 15}
	artGoldLight = [3]int{227, 201, 144}
	artGoldText  = [3]int{180, 138, 50}
)

// artworkLogo is the CAVE logo for the artwork, drawn once from its SVG at 1200 pixels wide (about 600 ppi as printed).
var artworkLogo = sync.OnceValues(func() ([]byte, error) {
	svg, err := staticFiles.ReadFile("static/logo-dark.svg")
	if err != nil {
		return nil, err
	}
	return logoImage(svg)
})

// The signatories under the two signatures.
var artSignatories = [2][]string{
	{"ADAM BIN YUSUF", "HRD Corp Accredited Trainer ID: 41123", "FOUNDER & CHIEF CONSULTANT", "THE CAVE CONSULT"},
	{"DINOR SABUDIN", "HRDC Trainer ID: 37066"},
}

// The awarding paragraph, as the sample sets its lines, with its typos fixed.
var artAward = []string{
	"This achievement is awarded under the",
	"Professional Competency Credential (PCC)",
	"in recognition of the knowledge, skills, and ability to apply",
	"the competencies demonstrated throughout the programme",
}

// sweep is one gold corner of the frame: the corner out to a curve from (0, h) to (w, 0), as a polygon. The preview
// draws the same curve as the path "M0 0V{h}C{.05w} {.45h} {.45w} {.04h} {w} 0Z".
func sweep(w, h float64) []fpdf.PointType {
	pts := []fpdf.PointType{{X: 0, Y: 0}}
	for i := 0; i <= 48; i++ {
		t := float64(i) / 48
		u := 1 - t
		pts = append(pts, fpdf.PointType{
			X: 3*u*u*t*.05*w + 3*u*t*t*.45*w + t*t*t*w,
			Y: u*u*u*h + 3*u*u*t*.45*h + 3*u*t*t*.04*h,
		})
	}
	return pts
}

// drawArtwork draws the built-in artwork's constant layer on the page.
func drawArtwork(pdf *fpdf.Fpdf) error {
	pageW, pageH := pdf.GetPageSize()
	k := pdf.GetConversionRatio()
	fill := func(c [3]int) { pdf.SetFillColor(c[0], c[1], c[2]) }
	draw := func(c [3]int) { pdf.SetDrawColor(c[0], c[1], c[2]) }
	ink := func(c [3]int) { pdf.SetTextColor(c[0], c[1], c[2]) }
	// spaced writes s centred on x with its baseline at y and track mm between letters, and returns its width.
	spaced := func(x, y, track float64, family, style string, size float64, s string) float64 {
		pdf.SetFont(family, style, size)
		w := pdf.GetStringWidth(s) + track*float64(len(s)-1)
		pdf.RawWriteStr(fmt.Sprintf("%.3f Tc", track*k))
		pdf.Text(x-w/2, y, s)
		pdf.RawWriteStr("0 Tc")
		return w
	}
	image := func(name string, x, y, w, h float64) error {
		f, err := staticFiles.Open("static/" + name)
		if err != nil {
			return err
		}
		defer f.Close()
		opt := fpdf.ImageOptions{ImageType: "png"}
		pdf.RegisterImageOptionsReader(name, opt, f)
		pdf.ImageOptions(name, x, y, w, h, false, opt, 0, "")
		return pdf.Error()
	}

	// The watermark: the logo up the right-hand side, faint, under everything else.
	logo, err := artworkLogo()
	if err != nil {
		return err
	}
	opt := fpdf.ImageOptions{ImageType: "png"}
	pdf.RegisterImageOptionsReader("logo-dark", opt, bytes.NewReader(logo))
	pdf.SetAlpha(.07, "Normal")
	pdf.TransformBegin()
	pdf.TransformRotate(90, 160, 150)
	pdf.ImageOptions("logo-dark", 85, 116.3, 150, 67.4, false, opt, 0, "")
	pdf.TransformEnd()
	pdf.SetAlpha(1, "Normal")

	// The frame: a double gold rule round the page, under two gold sweeps in opposite corners.
	draw(artGold)
	pdf.SetLineWidth(.35)
	pdf.Rect(5, 5, pageW-10, pageH-10, "D")
	pdf.SetLineWidth(.15)
	pdf.Rect(6.4, 6.4, pageW-12.8, pageH-12.8, "D")
	corner := func(w1, h1, w2, h2 float64) {
		pdf.ClipPolygon(sweep(w1, h1), false)
		pdf.LinearGradient(0, 0, w1, h1, artGoldDeep[0], artGoldDeep[1], artGoldDeep[2], artGold[0], artGold[1], artGold[2], 0, 1, .6, .4)
		pdf.ClipEnd()
		pdf.ClipPolygon(sweep(w2, h2), false)
		pdf.LinearGradient(0, 0, w2, h2, artGold[0], artGold[1], artGold[2], artGoldLight[0], artGoldLight[1], artGoldLight[2], 0, 1, .7, .3)
		pdf.ClipEnd()
		pdf.SetDrawColor(255, 255, 255)
		pdf.SetLineWidth(.6)
		pdf.MoveTo(0, h2)
		pdf.CurveBezierCubicTo(.05*w2, .45*h2, .45*w2, .04*h2, w2, 0)
		pdf.DrawPath("D")
	}
	corner(70, 82, 57, 67)
	pdf.TransformBegin()
	pdf.TransformRotate(180, pageW/2, pageH/2)
	corner(62, 112, 50, 96)
	pdf.TransformEnd()

	pdf.ImageOptions("logo-dark", 79, 17, 52, 0, false, opt, 0, "")

	ink(artInk)
	spaced(certNumberX, 15, .2, "Helvetica", "B", 7.5, "CERTIFICATE NO.")

	spaced(pageW/2, 63.6, 1.6, "Times", "", 40, "CERTIFICATE")
	ink(artGoldText)
	w := spaced(pageW/2, 72.7, .9, "Times", "", 20, "OF COMPETENCY")
	draw(artGold)
	pdf.SetLineWidth(.5)
	pdf.Line(45, 70.4, pageW/2-w/2-4, 70.4)
	pdf.Line(pageW/2+w/2+4, 70.4, pageW-45, 70.4)
	ink(artInk2)
	spaced(pageW/2, 88, .6, "Times", "", 11, "THIS IS TO CERTIFY THAT")

	// The rule the name and IC stand on.
	pdf.SetLineWidth(.3)
	pdf.Line(19, 112.5, pageW-19, 112.5)
	spaced(pageW/2, 119.5, 0, "Times", "", 11, "has successfully completed the competency requirements of")

	// The ornament under the Course title: two rules and a diamond between them.
	const orn = 157
	pdf.Line(30.5, orn, pageW/2-6, orn)
	pdf.Line(pageW/2+6, orn, pageW-30.5, orn)
	fill(artGold)
	pdf.Polygon([]fpdf.PointType{{X: pageW / 2, Y: orn - 1.8}, {X: pageW/2 + 2.6, Y: orn}, {X: pageW / 2, Y: orn + 1.8}, {X: pageW/2 - 2.6, Y: orn}}, "F")
	pdf.Circle(pageW/2-4.4, orn, .55, "F")
	pdf.Circle(pageW/2+4.4, orn, .55, "F")

	ink(artInk)
	for i, l := range artAward {
		spaced(pageW/2, orn+8.5+float64(i)*5.65, 0, "Times", "I", 13, l)
	}

	for _, im := range []struct {
		name       string
		x, y, w, h float64
	}{
		{"cert-seal.png", 10.5, 221, 51, 50.4},
		{"cert-hrd-corp.png", 69.6, 227.5, 31, 30.1},
		{"cert-signature-1.png", 62.8, 260, 48.2, 12.9},
		{"cert-signature-2.png", 128.6, 259.6, 24.8, 15.3},
	} {
		if err := image(im.name, im.x, im.y, im.w, im.h); err != nil {
			return err
		}
	}
	for i, x := range []float64{85.7, 141} {
		for j, l := range artSignatories[i] {
			size := 9.5
			if j == 0 || j == 3 {
				size = 11
			}
			spaced(x, 276.7+float64(i)*1.3+float64(j)*3.8, 0, "Times", "", size, l)
		}
	}
	return pdf.Error()
}

// certNameSize is the size in points the Learner's name prints at: 28, or smaller so a long name fits its line.
func certNameSize(pdf *fpdf.Fpdf, name string) float64 {
	pdf.SetFont("Times", "B", 28)
	return min(28, 28*certNameW/pdf.GetStringWidth(pdf.UnicodeTranslatorFromDescriptor("")(name)))
}

// badgeCutouts are the PCC badges as the Certificate prints them, PNGs keyed by static file name: each JPEG with the
// flat grey it was shot on made transparent, so the badge sits on the paper with no box round it.
var badgeCutouts = sync.OnceValues(func() (map[string][]byte, error) {
	names, err := fs.Glob(staticFiles, "static/pcc-*.jpg")
	if err != nil {
		return nil, err
	}
	cut := map[string][]byte{}
	for _, name := range names {
		f, err := staticFiles.Open(name)
		if err != nil {
			return nil, err
		}
		img, err := jpeg.Decode(f)
		f.Close()
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, cutout(img)); err != nil {
			return nil, err
		}
		cut[strings.TrimPrefix(name, "static/")] = buf.Bytes()
	}
	return cut, nil
})

// cutout makes a badge's grey surround transparent: the light, colourless pixels reachable from its edges, so the
// white panel inside the shield's gold rim stays. Each is unmixed from the grey (GIMP's colour to alpha), so the
// shield's soft shadow and the anti-aliased edges of the label keep their shading.
func cutout(img image.Image) *image.NRGBA {
	const grey = 252
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(out, out.Rect, img, b.Min, draw.Src)
	seen := make([]bool, w*h)
	var stack []int
	for x := range w {
		stack = append(stack, x, (h-1)*w+x)
	}
	for y := range h {
		stack = append(stack, y*w, y*w+w-1)
	}
	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		p := out.Pix[i*4 : i*4+4]
		lo, hi := min(p[0], p[1], p[2]), max(p[0], p[1], p[2])
		if seen[i] || lo < 120 || hi-lo > 40 {
			continue
		}
		seen[i] = true
		x, y := i%w, i/w
		a := float64(grey-min(int(lo), grey)) / grey
		if min(x, y, w-1-x, h-1-y) < 4 {
			a = 0 // the grey frame line round the edge of some badge files
		}
		for c := range 3 {
			if a > 0 {
				p[c] = uint8(max(0, min(255, (float64(p[c])-grey*(1-a))/a)))
			}
		}
		p[3] = uint8(a * 255)
		if x > 0 {
			stack = append(stack, i-1)
		}
		if x < w-1 {
			stack = append(stack, i+1)
		}
		if i >= w {
			stack = append(stack, i-w)
		}
		if i < (h-1)*w {
			stack = append(stack, i+w)
		}
	}
	return out
}

// certLine is one line of a Course title as the Certificate prints it: its text and baseline, in mm.
type certLine struct {
	Text string
	Y    float64
}

// certTitleLines wraps a Course title for the Certificate's title box at the largest size, down to 8 pt, whose lines
// fit it (below that a title overflows the box), and centres the lines in the box. size is in points.
func certTitleLines(pdf *fpdf.Fpdf, title string) (size float64, lines []certLine) {
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	words := strings.Fields(title)
	var texts []string
	for size = 16; ; size-- {
		pdf.SetFont("Times", "B", size)
		texts = texts[:0]
		for _, w := range words {
			if n := len(texts); n > 0 && pdf.GetStringWidth(tr(texts[n-1]+" "+w)) <= certTitleW {
				texts[n-1] += " " + w
			} else {
				texts = append(texts, w)
			}
		}
		if size <= 8 || float64(len(texts))*size*0.45 <= certTitleH {
			break
		}
	}
	lineH := size * 0.45 // 1.27 times the font size, in mm
	top := certTitleY + (certTitleH-float64(len(texts))*lineH)/2
	for i, t := range texts {
		lines = append(lines, certLine{Text: t, Y: top + float64(i)*lineH + size*0.3})
	}
	return size, lines
}

// certPreview is what the certificate-preview template draws: the artwork with a name, a Course title and a PCC
// badge, never an IC. Number and Issued are set only for an issued Certificate.
type certPreview struct {
	Name, Badge    string
	Title          []certLine
	NameSize       float64 // in mm
	TitleSize      float64 // in mm
	Number, Issued string
}

// newCertPreview is the template function certPreview: the preview of a Certificate for name on the Course titled title
// with the PCC badge static file badge ("" for none).
func newCertPreview(name, title, badge string) certPreview {
	pdf := fpdf.New("P", "mm", "A4", "")
	size, lines := certTitleLines(pdf, title)
	return certPreview{Name: name, Badge: badge, Title: lines, NameSize: certNameSize(pdf, name) * 25.4 / 72,
		TitleSize: size * 25.4 / 72}
}

// Preview is the Certificate's preview, as issued.
func (c certificate) Preview() certPreview {
	p := newCertPreview(c.LearnerName, c.CourseTitle, c.Badge())
	p.Number, p.Issued = c.Number(), c.IssuedIn("en")
	return p
}
