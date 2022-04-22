package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/JoshVarga/svgparser"
	"github.com/fogleman/triangulate"
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
	X, Y float64
}
type Color struct {
	R, G, B, A float64
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

func parseHashColor(col string) (c Color, err error) {
	matches := colorHashParser.FindStringSubmatch(col)

	if matches[0] == "" {
		err = fmt.Errorf("Uknown color format for '%s'", col)
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
	Fill      Color // replace with some sort of color
	Exterior  []Point
	Triangles []Triangle
}

func chompFloat(s string) (f float64, rem string) {
	dex := floatParser.FindStringSubmatchIndex(s)

	// dex[6] is the end
	var err error
	f, err = strconv.ParseFloat(s[dex[2]:dex[6]], 64)
	// TODO: ignore the error once the regex is debugged
	if err != nil {
		panic(err)
	}
	return f, s[dex[6]:]
}

type Bezier struct {
	p0, p1, c0, c1 Point
}

func (b Bezier) at(t float64) Point {
	a0 := Point{X: b.p0.X*t + b.c0.X*(1-t), Y: b.p0.Y*t + b.c0.Y*(1-t)}
	a1 := Point{X: b.c0.X*t + b.c1.X*(1-t), Y: b.c0.Y*t + b.c1.Y*(1-t)}
	a2 := Point{X: b.c1.X*t + b.p1.X*(1-t), Y: b.c1.Y*t + b.p1.Y*(1-t)}

	b0 := Point{X: a0.X*t + a1.X*(1-t), Y: a0.Y*t + a1.Y*(1-t)}
	b1 := Point{X: a1.X*t + a2.X*(1-t), Y: a1.Y*t + a2.Y*(1-t)}

	return Point{X: b0.X*t + b1.X*(1-t), Y: b0.X*t + b1.X*(1-t)}
}

func PolygonFromPathElement(el *svgparser.Element, bezierIncrement float64) (*Polygon, error) {
	if bezierIncrement <= 0 {
		panic(fmt.Errorf("negative bezier increment"))
	}
	var poly Polygon

	var tp triangulate.Polygon
	first := true
	finished := false

	b := Bezier{}

	d := el.Attributes["d"]
	d = strings.TrimLeft(d, " \n\r")
	var cmd byte
	x, y := 0., 0.
	dx, dy := 0., 0.
	c := [6]float64{}
	for len(d) > 0 {
		cmd, d = d[0], d[1:]
		switch cmd {
		case 'M':
			if !first {
				return nil, fmt.Errorf("move was not the first command")
			}
			x, d = chompFloat(d)
			d = strings.TrimLeft(d, " ,")
			y, d = chompFloat(d)
			d = strings.TrimLeft(d, " ")
			tp.Exterior = append(tp.Exterior, triangulate.Point{X: x, Y: y})
			fmt.Printf("x: %v, y: %v\n", x, y)
		case 'm':
			if !first {
				return nil, fmt.Errorf("move was not the first command")
			}
			dx, d = chompFloat(d)
			d = strings.TrimLeft(d, " ,\n\r")
			dy, d = chompFloat(d)
			d = strings.TrimLeft(d, " \n\r")
			x += dx
			y += dy
			tp.Exterior = append(tp.Exterior, triangulate.Point{X: x, Y: y})

			fmt.Printf("dx: %v, dy: %v\n", dx, dy)
		case 'L':
			x, d = chompFloat(d)
			d = strings.TrimLeft(d, " ,\n\r")
			y, d = chompFloat(d)
			d = strings.TrimLeft(d, " ,\n\r")

			tp.Exterior = append(tp.Exterior, triangulate.Point{X: x, Y: y})
		case 'l':
			dx, d = chompFloat(d)
			d = strings.TrimLeft(d, " ,\n\r")
			dy, d = chompFloat(d)
			d = strings.TrimLeft(d, " ,\n\r")
			x += dx
			y += dy

			tp.Exterior = append(tp.Exterior, triangulate.Point{X: x, Y: y})
		case 'V':
			y, d = chompFloat(d)
			d = strings.TrimLeft(d, " \n\r")

			tp.Exterior = append(tp.Exterior, triangulate.Point{X: x, Y: y})
		case 'v':
			dy, d = chompFloat(d)
			d = strings.TrimLeft(d, " \n\r")
			y += dy

			tp.Exterior = append(tp.Exterior, triangulate.Point{X: x, Y: y})
		case 'H':
			x, d = chompFloat(d)
			d = strings.TrimLeft(d, " \n\r")

			tp.Exterior = append(tp.Exterior, triangulate.Point{X: x, Y: y})
		case 'h':
			dx, d = chompFloat(d)
			d = strings.TrimLeft(d, " \n\r")
			x += dx

			tp.Exterior = append(tp.Exterior, triangulate.Point{X: x, Y: y})
		case 'C':
			for i, _ := range c {
				c[i], d = chompFloat(d)
				d = strings.TrimLeft(d, " ,\n\r")
			}
			b.p0.X, b.p0.Y = x, y
			b.c0.X, b.c0.Y = c[0], c[1]
			b.c1.X, b.c1.Y = c[2], c[3]
			b.p1.X, b.p1.Y = c[4], c[5]

			for eps := 0.; eps < 1.0; eps += bezierIncrement {
				pp := b.at(eps)
				tp.Exterior = append(tp.Exterior, triangulate.Point{X: pp.X, Y: pp.Y})
			}
			tp.Exterior = append(tp.Exterior, triangulate.Point{X: c[4], Y: c[5]})
		case 'c':
			for i, _ := range c {
				c[i], d = chompFloat(d)
				d = strings.TrimLeft(d, " ,\n\r")
			}
			b.p0.X, b.p0.Y = x, y
			b.c0.X, b.c0.Y = x+c[0], y+c[1]
			b.c1.X, b.c1.Y = x+c[2], y+c[3]
			b.p1.X, b.p1.Y = x+c[4], y+c[5]

			for eps := 0.; eps < 1.0; eps += bezierIncrement {
				pp := b.at(eps)
				tp.Exterior = append(tp.Exterior, triangulate.Point{X: pp.X, Y: pp.Y})
			}
			tp.Exterior = append(tp.Exterior, triangulate.Point{X: x + c[4], Y: y + c[5]})
		case 'Z':
			fallthrough
		case 'z':
			finished = true
		}

		if first {
			first = false
		}
	}

	if !finished {
		return &poly, fmt.Errorf("I only know how to handle z terminated paths right now")
	}
	d = strings.TrimLeft(d, " \n\r")
	if d != "" {
		return &poly, fmt.Errorf("Z wasn't the last command")
	}

	for _, p := range tp.Exterior {
		poly.Exterior = append(poly.Exterior, Point{X: p.X, Y: p.Y})
	}

	indices := make(map[triangulate.Point]int)
	for i := 0; i < len(tp.Exterior); i++ {
		indices[tp.Exterior[i]] = i
	}

	tris := tp.Triangulate()

	if el.Attributes["fill"] != "" {
		poly.Fill = MustParseColor(el.Attributes["fill"])
	}
	for _, t := range tris {
		poly.Triangles = append(poly.Triangles, [3]int{
			indices[t.A], indices[t.B], indices[t.C],
		})
	}

	fmt.Printf("d: %s\n", d)

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
	poly.Triangles = []Triangle{
		{0, 1, 2},
		{1, 2, 3},
	}
	if el.Attributes["fill"] != "" {
		poly.Fill = MustParseColor(el.Attributes["fill"])
	}

	return &poly, nil
}

func PolygonFromPolygonElement(el *svgparser.Element) (*Polygon, error) {
	var poly triangulate.Polygon
	coords := coordsSplitter.Split(el.Attributes["points"], -1)
	var ret Polygon

	// fmt.Printf("coords: %v", coords)

	for i := 0; i+1 < len(coords); i += 2 {
		// fmt.Printf("coords: %s %s", coords[i], coords[i+1])
		if x, err := strconv.ParseFloat(coords[i], 64); err != nil {
			return nil, err
		} else if y, err := strconv.ParseFloat(coords[i+1], 64); err != nil {
			return nil, err
		} else {
			// indicies are the same
			poly.Exterior = append(poly.Exterior, triangulate.Point{X: x, Y: y})
			ret.Exterior = append(ret.Exterior, Point{X: x, Y: y})
		}
	}

	indices := make(map[triangulate.Point]int)
	for i := 0; i < len(poly.Exterior); i++ {
		indices[poly.Exterior[i]] = i
	}

	tris := poly.Triangulate()

	if el.Attributes["fill"] != "" {
		ret.Fill = MustParseColor(el.Attributes["fill"])
	}
	for _, t := range tris {
		ret.Triangles = append(ret.Triangles, [3]int{
			indices[t.A], indices[t.B], indices[t.C],
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
			if poly, err := PolygonFromPathElement(el, 0.05); err != nil {
				return ret, err
			} else {
				ret = append(ret, *poly)
			}
		}

		stack = append(stack, el.Children...)
	}
	return
}

func main() {
	fmt.Println("Hello World!")

	path := "../ItsFive/svg/countries/belarus.svg"

	country, err := os.Open(path)
	if err != nil {
		panic(fmt.Errorf("error opening file: %v", err))
	}
	elements, err := svgparser.Parse(country, false)
	if err != nil {
		panic(fmt.Errorf("error parsing svg '%s': %v", err, path))
	}

	fmt.Printf("elements: %v\n", elements)

	polys, err := ExtractPolygons(elements)
	if err != nil {
		panic(err)
	}

	fmt.Printf("tris: %v\n", polys)
}
