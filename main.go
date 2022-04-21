package main

import (
	"fmt"
	"os"
	"regexp"
	"strconv"

	"github.com/JoshVarga/svgparser"
	"github.com/fogleman/triangulate"
)

type Point struct {
	X, Y float64
}
type Triangle [3]int

type Polygon struct {
	Fill      string // replace with some sort of color
	Exterior  []Point
	Triangles []Triangle
}

var (
	coordsSplitter *regexp.Regexp
)

func init() {
	coordsSplitter = regexp.MustCompile(`[\s,]+`)
}

func PolygonFromElement(el *svgparser.Element) (*Polygon, error) {
	if el.Name != "polygon" {
		return nil, fmt.Errorf("expected <polygon> got <%s>", el.Name)
	}

	var poly triangulate.Polygon
	coords := coordsSplitter.Split(el.Attributes["points"], -1)
	var ret Polygon

	fmt.Printf("coords: %v", coords)

	for i := 0; i+1 < len(coords); i += 2 {
		fmt.Printf("coords: %s %s", coords[i], coords[i+1])
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

	ret.Fill = el.Attributes["fill"]
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

		if el.Name == "polygon" {
			if poly, err := PolygonFromElement(el); err != nil {
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

	path := "/home/donniet/src/ItsFive/svg/countries/belarus.svg"

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
