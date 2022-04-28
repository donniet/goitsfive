package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"unicode"

	"github.com/JoshVarga/svgparser"
	"github.com/tchayen/triangolatte"
	"golang.org/x/exp/slices"
)

var (
	coordsSplitter, colorHashParser, floatParser *regexp.Regexp
)

func init() {
	coordsSplitter = regexp.MustCompile(`[\s,]+`)
	colorHashParser = regexp.MustCompile(`^#([0-9A-Fa-f]{6})|([0-9A-Fa-f]{3})$`)
	floatParser = regexp.MustCompile(`^([+-]?([0-9]*[.])?[0-9]+)([^0-9.]|$)`)
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (p Point) Add(q Point) Point {
	return Point{X: p.X + q.X, Y: p.Y + q.Y}
}
func (p Point) Equals(q Point) bool {
	return p.X == q.X && p.Y == q.Y
}

type Ring []Point

func (r Ring) At(i int) Point {
	if len(r) == 0 {
		return Point{}
	}
	return r[i%len(r)]
}
func (r Ring) Length() int {
	return len(r)
}
func (r Ring) Area() (area float64) {
	if len(r) <= 2 {
		return 0
	}

	p0 := r.At(0)
	for i := 1; i <= len(r); i++ {
		p1 := r.At(i)
		area += p0.X*p1.Y - p1.X*p0.Y
		p0 = p1
	}
	return
}

type Bezier struct {
	p0, p1, c0, c1 Point
}

func (b Bezier) at(t float64) Point {
	a0 := Point{X: b.p0.X*(1-t) + b.c0.X*t, Y: b.p0.Y*(1-t) + b.c0.Y*t}
	a1 := Point{X: b.c0.X*(1-t) + b.c1.X*t, Y: b.c0.Y*(1-t) + b.c1.Y*t}
	a2 := Point{X: b.c1.X*(1-t) + b.p1.X*t, Y: b.c1.Y*(1-t) + b.p1.Y*t}

	b0 := Point{X: a0.X*(1-t) + a1.X*t, Y: a0.Y*(1-t) + a1.Y*t}
	b1 := Point{X: a1.X*(1-t) + a2.X*t, Y: a1.Y*(1-t) + a2.Y*t}

	return Point{X: b0.X*(1-t) + b1.X*t, Y: b0.Y*(1-t) + b1.Y*t}
}

type Color struct {
	R, G, B, A float64
}

type SVGDReader struct {
	io.RuneScanner
}
type SVGDCommand rune

const (
	SVGDInvalidCommand            SVGDCommand = 0
	SVGDAbsoluteMoveCommand       SVGDCommand = 'M'
	SVGDRelativeMoveCommand       SVGDCommand = 'm'
	SVGDAbsoluteLineCommand       SVGDCommand = 'L'
	SVGDRelativeLineCommand       SVGDCommand = 'l'
	SVGDAbsoluteVerticalCommand   SVGDCommand = 'V'
	SVGDRelativeVerticalCommand   SVGDCommand = 'v'
	SVGDAbsoluteHorizontalCommand SVGDCommand = 'H'
	SVGDRelativeHorizontalCommand SVGDCommand = 'h'
	SVGDAbsoluteCurveCommand      SVGDCommand = 'C'
	SVGDRelativeCurveCommand      SVGDCommand = 'c'
	SVGDAbsoluteCloseCommand      SVGDCommand = 'Z'
	SVGDRelativeCloseCommand      SVGDCommand = 'z'
)

var (
	SVGAllCommands = []rune{
		rune(SVGDAbsoluteMoveCommand), rune(SVGDRelativeMoveCommand), rune(SVGDAbsoluteLineCommand), rune(SVGDRelativeLineCommand),
		rune(SVGDAbsoluteVerticalCommand), rune(SVGDRelativeVerticalCommand),
		rune(SVGDAbsoluteHorizontalCommand), rune(SVGDRelativeHorizontalCommand), rune(SVGDAbsoluteCurveCommand), rune(SVGDRelativeCurveCommand),
		rune(SVGDAbsoluteCloseCommand), rune(SVGDRelativeCloseCommand),
	}
)

func (r SVGDReader) ChompCommand() (SVGDCommand, error) {
	if ru, _, err := r.RuneScanner.ReadRune(); err != nil {
		return SVGDInvalidCommand, err
	} else if slices.Index(SVGAllCommands, ru) >= 0 {
		return SVGDCommand(ru), nil
	} else if err := r.RuneScanner.UnreadRune(); err != nil {
		return SVGDInvalidCommand, fmt.Errorf("could not unread rune: %v", err)
	}
	return SVGDInvalidCommand, fmt.Errorf("invalid reader state: no valid command found")
}

type SVGDPart interface {
	Linearize(start Point, res float64) []Point
}

type SVGDAbsoluteMovePart struct {
	Point
}

// TODO: how to handle multple paths
func (p SVGDAbsoluteMovePart) Linearize(start Point, res float64) []Point {
	return []Point{p.Point}
}

type SVGDRelativeMovePart struct {
	Point
}

// TODO: how to handle multple paths
func (p SVGDRelativeMovePart) Linearize(start Point, res float64) []Point {
	return []Point{start.Add(p.Point)}
}

type SVGDAbsoluteLinePart struct {
	Point
}

func (p SVGDAbsoluteLinePart) Linearize(start Point, res float64) []Point {
	return []Point{p.Point}
}

type SVGDRelativeLinePart struct {
	Point
}

func (p SVGDRelativeLinePart) Linearize(start Point, res float64) []Point {
	return []Point{start.Add(p.Point)}
}

type SVGDAbsoluteHorizontalPart struct {
	distance float64
}

func (p SVGDAbsoluteHorizontalPart) Linearize(start Point, res float64) []Point {
	return []Point{{X: p.distance, Y: start.Y}}
}

type SVGDRelativeHorizontalPart struct {
	distance float64
}

func (p SVGDRelativeHorizontalPart) Linearize(start Point, res float64) []Point {
	return []Point{start.Add(Point{X: p.distance, Y: 0})}
}

type SVGDAbsoluteVerticalPart struct {
	distance float64
}

func (p SVGDAbsoluteVerticalPart) Linearize(start Point, res float64) []Point {
	return []Point{{X: start.X, Y: p.distance}}
}

type SVGDRelativeVerticalPart struct {
	distance float64
}

func (p SVGDRelativeVerticalPart) Linearize(start Point, res float64) []Point {
	return []Point{start.Add(Point{X: 0, Y: p.distance})}
}

type SVGDAbsoluteCurvePart struct {
	points [3]Point
}

func (p SVGDAbsoluteCurvePart) Linearize(start Point, res float64) (ret []Point) {
	b := Bezier{p0: start, c0: p.points[0], c1: p.points[1], p1: p.points[2]}
	for e := 0.; e < 1.0; e += res {
		ret = append(ret, b.at(e))
	}
	ret = append(ret, b.at(1.))
	return
}

type SVGDRelativeCurvePart struct {
	points [3]Point
}

func (p SVGDRelativeCurvePart) Linearize(start Point, res float64) (ret []Point) {
	b := Bezier{p0: start, c0: start.Add(p.points[0]), c1: start.Add(p.points[1]), p1: start.Add(p.points[2])}
	for e := 0.; e < 1.0; e += res {
		ret = append(ret, b.at(e))
	}
	ret = append(ret, b.at(1.))
	return
}

type SVGDClosePart struct{}

func (p SVGDClosePart) Linearize(start Point, res float64) (ret []Point) {
	return
}

func MakePart(cmd SVGDCommand, coords ...float64) (SVGDPart, error) {
	switch cmd {
	case SVGDAbsoluteMoveCommand:
		return SVGDAbsoluteMovePart{Point: Point{X: coords[0], Y: coords[1]}}, nil
	case SVGDRelativeMoveCommand:
		return SVGDRelativeMovePart{Point: Point{X: coords[0], Y: coords[1]}}, nil
	case SVGDAbsoluteLineCommand:
		return SVGDAbsoluteLinePart{Point: Point{X: coords[0], Y: coords[1]}}, nil
	case SVGDRelativeLineCommand:
		return SVGDRelativeLinePart{Point: Point{X: coords[0], Y: coords[1]}}, nil
	case SVGDAbsoluteHorizontalCommand:
		return SVGDAbsoluteHorizontalPart{distance: coords[0]}, nil
	case SVGDRelativeHorizontalCommand:
		return SVGDRelativeHorizontalPart{distance: coords[0]}, nil
	case SVGDAbsoluteVerticalCommand:
		return SVGDAbsoluteVerticalPart{distance: coords[0]}, nil
	case SVGDRelativeVerticalCommand:
		return SVGDRelativeVerticalPart{distance: coords[0]}, nil
	case SVGDAbsoluteCurveCommand:
		return SVGDAbsoluteCurvePart{points: [3]Point{
			{X: coords[0], Y: coords[1]},
			{X: coords[2], Y: coords[3]},
			{X: coords[4], Y: coords[5]},
		}}, nil
	case SVGDRelativeCurveCommand:
		return SVGDRelativeCurvePart{points: [3]Point{
			{X: coords[0], Y: coords[1]},
			{X: coords[2], Y: coords[3]},
			{X: coords[4], Y: coords[5]},
		}}, nil
	case SVGDAbsoluteCloseCommand:
		fallthrough
	case SVGDRelativeCloseCommand:
		return SVGDClosePart{}, nil
	}

	return nil, fmt.Errorf("invalid coordinates for part")
}

type SVGDParts []SVGDPart

func (a SVGDParts) Linearize(res float64) (ret []Point) {
	for _, p := range a {
		last := Point{}
		if e := len(ret) - 1; e >= 0 {
			last = ret[e]
		}

		ret = append(ret, p.Linearize(last, res)...)
	}
	return
}

func (r SVGDReader) Parse() (parts SVGDParts, err error) {
	cmd := SVGDInvalidCommand
	var part SVGDPart
	x, y := 0., 0.
	c := make([]float64, 6)
	for {
		if _, err = r.ChompSeperator(); err != nil {
			//TODO: check for the end of the stream
			return
		} else if cmd, err = r.ChompCommand(); err != nil {
			return
		}

		switch cmd {
		case SVGDAbsoluteLineCommand:
			fallthrough
		case SVGDRelativeLineCommand:
			fallthrough
		case SVGDAbsoluteMoveCommand:
			fallthrough
		case SVGDRelativeMoveCommand:
			if x, err = r.ChompNumber(); err != nil {
				return
			} else if _, err = r.ChompSeperator(); err != nil {
				return
			} else if y, err = r.ChompNumber(); err != nil {
				return
			} else if part, err = MakePart(cmd, x, y); err != nil {
				return
			}
			parts = append(parts, part)
		case SVGDAbsoluteHorizontalCommand:
			fallthrough
		case SVGDRelativeHorizontalCommand:
			fallthrough
		case SVGDAbsoluteVerticalCommand:
			fallthrough
		case SVGDRelativeVerticalCommand:
			if x, err = r.ChompNumber(); err != nil {
				return
			} else if part, err = MakePart(cmd, x); err != nil {
				return
			}
			parts = append(parts, part)
		case SVGDAbsoluteCurveCommand:
			fallthrough
		case SVGDRelativeCurveCommand:
			for i := range c {
				if c[i], err = r.ChompNumber(); err != nil {
					return
				} else if _, err = r.ChompSeperator(); err != nil {
					return
				}
			}
			if part, err = MakePart(cmd, c...); err != nil {
				return
			}
			parts = append(parts, part)
		case SVGDAbsoluteCloseCommand:
			fallthrough
		case SVGDRelativeCloseCommand:
			if part, err = MakePart(cmd); err != nil {
				return
			}
			parts = append(parts, part)
			return
		}
	}
}

// returns -1.0, 1.0 or 0 on error
func (r SVGDReader) ChompSign() (float64, error) {
	if ru, _, err := r.RuneScanner.ReadRune(); err != nil {
		return 0, err
	} else if ru == '+' {
		return 1, nil
	} else if ru == '-' {
		return -1, nil
	} else if ru == '.' || (ru >= '0' && ru <= '9') {
		// assume positive if there is a number after
		if err := r.RuneScanner.UnreadRune(); err != nil {
			return 0, err
		}
		return 1, nil
	}
	return 0, fmt.Errorf("not a number")
}

func (r SVGDReader) ChompSeperator() (string, error) {
	var str []rune
	for {
		if ru, _, err := r.RuneScanner.ReadRune(); err != nil {
			return string(str), err
		} else if unicode.IsSpace(ru) || ru == ',' {
			str = append(str, ru)
		} else if err := r.RuneScanner.UnreadRune(); err != nil {
			return string(str), err
		} else {
			return string(str), nil
		}
	}
}

func (r SVGDReader) ChompNumber() (float64, error) {
	// first get the sign
	sign := 1.
	var err error
	if sign, err = r.ChompSign(); err != nil {
		return 0, err
	}

	// have we seen a decimal point?
	point := false
	var str []rune

	for {
		if ru, _, err := r.RuneScanner.ReadRune(); err != nil {
			return 0, err
		} else if ru == '.' {
			if point {
				return 0, fmt.Errorf("double decimal point detected")
			}
			str = append(str, ru)
			point = true
		} else if ru >= '0' && ru <= '9' {
			str = append(str, ru)
		} else if err := r.RuneScanner.UnreadRune(); err != nil {
			return 0, err
		} else {
			break
		}
	}

	if len(str) == 0 {
		return 0, fmt.Errorf("no number found")
	} else if num, err := strconv.ParseFloat(string(str), 64); err != nil {
		return 0, err
	} else {
		return sign * num, nil
	}
}

func mustParseHex(s string) (x uint64) {
	var err error
	if x, err = strconv.ParseUint(s, 16, 64); err != nil {
		panic(err)
	}
	return
}

func mustParseHexColor(s string) float64 {
	shifter := 1 << (4 * len(s))
	return float64(mustParseHex(s)) / float64(shifter)
}

func Reverse[K interface{}](s []K) {
	for i := 0; i < len(s)/2; i++ {
		j := len(s) - i - 1
		s[i], s[j] = s[j], s[i]
	}
}
func Map[K interface{}, V interface{}](s []K, t func(K) V) (r []V) {
	for _, k := range s {
		r = append(r, t(k))
	}
	return
}
func RemoveDuplicates[K interface{}](s []K, pred func(K, K) bool) (ret []K) {
	for _, k := range s {
		if dex := len(ret); dex > 0 && pred(k, ret[dex-1]) {
			continue
		}
		ret = append(ret, k)
	}
	return
}

func parseHashColor(col string) (c Color, err error) {
	matches := colorHashParser.FindStringSubmatch(col)

	if matches[0] == "" {
		err = fmt.Errorf("uknown color format for '%s'", col)
		return
	}

	if col := matches[2]; len(col) == 3 {
		c.R = mustParseHexColor(col[0:1])
		c.G = mustParseHexColor(col[1:2])
		c.B = mustParseHexColor(col[2:3])
		return
	} else if col := matches[1]; len(col) == 6 {
		c.R = mustParseHexColor(col[0:2])
		c.G = mustParseHexColor(col[2:4])
		c.B = mustParseHexColor(col[4:6])
		return
	}

	//TODO: remove after debugging
	panic(fmt.Errorf("check the colorHashParser regex because we should never get here"))
}

func ParseColor(col string) (Color, error) {
	//TODO: add RGB and RGBA colors
	return parseHashColor(col)
}

func MustParseColor(col string) Color {
	c, err := ParseColor(col)
	if err != nil {
		panic(err)
	}
	return c
}

type Triangle [3]int

type Polygon struct {
	Fill      Color      `json:"fill"` // replace with some sort of color
	Exterior  []Point    `json:"exterior"`
	Triangles []Triangle `json:"triangle"`
}

func PolygonFromPathElement(el *svgparser.Element, res float64) (*Polygon, error) {
	if res <= 0 {
		panic(fmt.Errorf("negative bezier increment"))
	}
	var poly Polygon

	var tp []triangolatte.Point

	d := el.Attributes["d"]

	fmt.Fprintf(os.Stderr, "d attribute: %s\n", d)

	dreader := SVGDReader{strings.NewReader(d)}

	parts, err := dreader.Parse()
	if err != nil {
		return nil, err
	}

	// reverse it
	// Reverse(tp)

	poly.Exterior = parts.Linearize(res)
	poly.Exterior = RemoveDuplicates(poly.Exterior, func(p, q Point) bool { return p.Equals(q) })
	fmt.Fprintf(os.Stderr, "area: %f\n", Ring(poly.Exterior).Area())
	if area := Ring(poly.Exterior).Area(); area < 0 {
		Reverse(poly.Exterior)
	}
	tp = Map(poly.Exterior, func(p Point) triangolatte.Point {
		return triangolatte.Point{X: p.X, Y: p.Y}
	})

	// for _, p := range poly.Exterior {
	// 	tp = append(tp, triangolatte.Point{X: p.X, Y: p.Y})
	// }

	indices := make(map[triangolatte.Point]int)
	for i := 0; i < len(tp); i++ {
		indices[tp[i]] = i
	}

	fmt.Fprintf(os.Stderr, "polys: %#v\n", poly)

	tris, err := triangolatte.Polygon(tp)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(os.Stderr, "tris: %#v\n", tris)

	if el.Attributes["fill"] != "" {
		poly.Fill = MustParseColor(el.Attributes["fill"])
	}
	for i := 0; i < len(tris); i += 6 {
		A := triangolatte.Point{X: tris[i+0], Y: tris[i+1]}
		B := triangolatte.Point{X: tris[i+2], Y: tris[i+3]}
		C := triangolatte.Point{X: tris[i+4], Y: tris[i+5]}

		poly.Triangles = append(poly.Triangles, [3]int{
			indices[A], indices[B], indices[C],
		})
	}

	// fmt.Printf("d: %s\n", d)

	return &poly, nil
}

func PolygonFromRectElement(el *svgparser.Element) (*Polygon, error) {
	var poly Polygon

	var x0, y0, x1, y1 float64
	var err error
	if x0, err = strconv.ParseFloat(el.Attributes["x"], 64); err != nil {
		return nil, err
	}
	if y0, err = strconv.ParseFloat(el.Attributes["y"], 64); err != nil {
		return nil, err
	}
	if x1, err = strconv.ParseFloat(el.Attributes["width"], 64); err != nil {
		return nil, err
	} else {
		x1 += x0
	}
	if y1, err = strconv.ParseFloat(el.Attributes["height"], 64); err != nil {
		return nil, err
	} else {
		y1 += y0
	}

	poly.Exterior = []Point{
		{X: x0, Y: y0},
		{X: x0, Y: y1},
		{X: x1, Y: y1},
		{X: x1, Y: y0},
	}
	//TODO: check right handed/v/left handed
	poly.Triangles = []Triangle{
		{0, 1, 2},
		{2, 3, 0},
	}
	if el.Attributes["fill"] != "" {
		poly.Fill = MustParseColor(el.Attributes["fill"])
	}

	return &poly, nil
}

func PolygonFromPolygonElement(el *svgparser.Element) (*Polygon, error) {
	var poly []triangolatte.Point
	coords := coordsSplitter.Split(el.Attributes["points"], -1)
	var ret Polygon

	// fmt.Printf("coords: %v", coords)
	fmt.Fprintf(os.Stderr, "coords: %v\n", coords)

	for i := 0; i+1 < len(coords); i += 2 {
		// fmt.Printf("coords: %s %s", coords[i], coords[i+1])
		if x, err := strconv.ParseFloat(coords[i], 64); err != nil {
			return nil, err
		} else if y, err := strconv.ParseFloat(coords[i+1], 64); err != nil {
			return nil, err
		} else {
			// indicies are the same
			ret.Exterior = append(ret.Exterior, Point{X: x, Y: y})
		}
	}

	if area := Ring(ret.Exterior).Area(); area < 0 {
		Reverse(ret.Exterior)
	}
	fmt.Fprintf(os.Stderr, "area: %f\n", Ring(ret.Exterior).Area())

	poly = Map(ret.Exterior, func(p Point) triangolatte.Point {
		return triangolatte.Point{X: p.X, Y: p.Y}
	})

	indices := make(map[triangolatte.Point]int)
	for i := 0; i < len(poly); i++ {
		indices[poly[i]] = i
	}

	// Reverse(poly)

	tris, err := triangolatte.Polygon(poly)
	if err != nil {
		return nil, err
	}

	if el.Attributes["fill"] != "" {
		ret.Fill = MustParseColor(el.Attributes["fill"])
	}
	for i := 0; i < len(tris); i += 6 {
		A := triangolatte.Point{X: tris[i+0], Y: tris[i+1]}
		B := triangolatte.Point{X: tris[i+2], Y: tris[i+3]}
		C := triangolatte.Point{X: tris[i+4], Y: tris[i+5]}

		ret.Triangles = append(ret.Triangles, [3]int{
			indices[A], indices[B], indices[C],
		})
	}

	return &ret, nil
}

func ExtractPolygons(el *svgparser.Element) (ret []Polygon, err error) {
	var stack []*svgparser.Element

	stack = append(stack, el)

	for len(stack) > 0 {
		el, stack = stack[len(stack)-1], stack[:len(stack)-1]

		switch el.Name {
		case "polygon":
			if poly, err := PolygonFromPolygonElement(el); err != nil {
				return ret, err
			} else {
				ret = append(ret, *poly)
			}
		case "rect":
			if poly, err := PolygonFromRectElement(el); err != nil {
				return ret, err
			} else {
				ret = append(ret, *poly)
			}
		case "path":
			if poly, err := PolygonFromPathElement(el, 0.1); err != nil {
				return ret, err
			} else {
				ret = append(ret, *poly)
			}
		}

		stack = append(stack, el.Children...)
	}
	return
}

func WriteOBJ(writer io.Writer, polys []Polygon) {
	firstVertex := make(map[int]int)
	count := 1
	for i, p := range polys {
		firstVertex[i] = count
		count += len(p.Exterior)

		for _, v := range p.Exterior {
			fmt.Fprintf(writer, "v %f %f 0\n", v.X, v.Y)
		}
	}

	// fmt.Print("f ")
	// v := 1
	// for _, p := range polys {
	// 	for _ = range p.Exterior {
	// 		fmt.Printf("%d ", v)
	// 		v++
	// 	}
	// }
	// fmt.Print("\n")

	for i, p := range polys {
		f := firstVertex[i]
		for _, t := range p.Triangles {
			fmt.Fprintf(writer, "f %d %d %d\n", f+t[0], f+t[1], f+t[2])
		}
	}

}

func main() {
	flag.Parse()
	svgPath := ""

	if flag.Arg(0) == "" {
		svgPath = "test.svg"
	} else {
		svgPath = flag.Arg(0)
	}

	country, err := os.Open(svgPath)
	if err != nil {
		panic(fmt.Errorf("error opening file: %v", err))
	}
	elements, err := svgparser.Parse(country, false)
	if err != nil {
		panic(fmt.Errorf("error parsing svg '%s': %v", err, svgPath))
	}

	polys, err := ExtractPolygons(elements)
	if err != nil {
		panic(err)
	}

	// WriteOBJ(os.Stdout, polys)

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "\t")
	encoder.Encode(polys)

	// fmt.Printf("tris: %v\n", polys)
}
