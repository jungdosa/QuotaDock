package ui

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/vector"
)

type svgPoint struct {
	X float64
	Y float64
}

type svgSegment struct {
	command byte
	points  [3]svgPoint
}

type svgSubpath struct {
	start    svgPoint
	segments []svgSegment
	closed   bool
}

type parsedSVGPath struct {
	subpaths []svgSubpath
}

type svgBounds struct {
	MinX  float64
	MinY  float64
	MaxX  float64
	MaxY  float64
	valid bool
}

func (b *svgBounds) include(point svgPoint) {
	if !b.valid {
		b.MinX, b.MinY, b.MaxX, b.MaxY = point.X, point.Y, point.X, point.Y
		b.valid = true
		return
	}
	b.MinX = min(b.MinX, point.X)
	b.MinY = min(b.MinY, point.Y)
	b.MaxX = max(b.MaxX, point.X)
	b.MaxY = max(b.MaxY, point.Y)
}

func (b *svgBounds) merge(other svgBounds) {
	if !other.valid {
		return
	}
	b.include(svgPoint{X: other.MinX, Y: other.MinY})
	b.include(svgPoint{X: other.MaxX, Y: other.MaxY})
}

func (p parsedSVGPath) bounds() svgBounds {
	bounds := svgBounds{}
	for _, subpath := range p.subpaths {
		current := subpath.start
		bounds.include(current)
		for _, segment := range subpath.segments {
			for step := 1; step <= 32; step++ {
				t := float64(step) / 32
				switch segment.command {
				case 'L':
					bounds.include(interpolatePoint(current, segment.points[0], t))
				case 'Q':
					bounds.include(quadraticPoint(current, segment.points[0], segment.points[1], t))
				case 'C':
					bounds.include(cubicPoint(current, segment.points[0], segment.points[1], segment.points[2], t))
				}
			}
			current = segmentEnd(segment)
		}
	}
	return bounds
}

func interpolatePoint(left, right svgPoint, t float64) svgPoint {
	return svgPoint{X: left.X + (right.X-left.X)*t, Y: left.Y + (right.Y-left.Y)*t}
}

func quadraticPoint(start, control, end svgPoint, t float64) svgPoint {
	u := 1 - t
	return svgPoint{
		X: u*u*start.X + 2*u*t*control.X + t*t*end.X,
		Y: u*u*start.Y + 2*u*t*control.Y + t*t*end.Y,
	}
}

func cubicPoint(start, control1, control2, end svgPoint, t float64) svgPoint {
	u := 1 - t
	return svgPoint{
		X: u*u*u*start.X + 3*u*u*t*control1.X + 3*u*t*t*control2.X + t*t*t*end.X,
		Y: u*u*u*start.Y + 3*u*u*t*control1.Y + 3*u*t*t*control2.Y + t*t*t*end.Y,
	}
}

func segmentEnd(segment svgSegment) svgPoint {
	switch segment.command {
	case 'C':
		return segment.points[2]
	case 'Q':
		return segment.points[1]
	default:
		return segment.points[0]
	}
}

type svgPathScanner struct {
	source string
	index  int
}

func (s *svgPathScanner) skipSeparators() {
	for s.index < len(s.source) {
		switch s.source[s.index] {
		case ' ', '\t', '\r', '\n', ',':
			s.index++
		default:
			return
		}
	}
}

func (s *svgPathScanner) hasNumber() bool {
	s.skipSeparators()
	if s.index >= len(s.source) {
		return false
	}
	candidate := s.source[s.index]
	return candidate == '+' || candidate == '-' || candidate == '.' || candidate >= '0' && candidate <= '9'
}

func (s *svgPathScanner) number() (float64, error) {
	s.skipSeparators()
	start := s.index
	if s.index < len(s.source) && (s.source[s.index] == '+' || s.source[s.index] == '-') {
		s.index++
	}
	digits := 0
	for s.index < len(s.source) && s.source[s.index] >= '0' && s.source[s.index] <= '9' {
		s.index++
		digits++
	}
	if s.index < len(s.source) && s.source[s.index] == '.' {
		s.index++
		for s.index < len(s.source) && s.source[s.index] >= '0' && s.source[s.index] <= '9' {
			s.index++
			digits++
		}
	}
	if digits == 0 {
		return 0, fmt.Errorf("expected number at byte %d", start)
	}
	if s.index < len(s.source) && (s.source[s.index] == 'e' || s.source[s.index] == 'E') {
		exponent := s.index
		s.index++
		if s.index < len(s.source) && (s.source[s.index] == '+' || s.source[s.index] == '-') {
			s.index++
		}
		exponentDigits := 0
		for s.index < len(s.source) && s.source[s.index] >= '0' && s.source[s.index] <= '9' {
			s.index++
			exponentDigits++
		}
		if exponentDigits == 0 {
			return 0, fmt.Errorf("invalid exponent at byte %d", exponent)
		}
	}
	value, err := strconv.ParseFloat(s.source[start:s.index], 64)
	if err != nil {
		return 0, fmt.Errorf("parse number %q: %w", s.source[start:s.index], err)
	}
	return value, nil
}

func (s *svgPathScanner) flag() (bool, error) {
	s.skipSeparators()
	if s.index >= len(s.source) || s.source[s.index] != '0' && s.source[s.index] != '1' {
		return false, fmt.Errorf("expected arc flag at byte %d", s.index)
	}
	value := s.source[s.index] == '1'
	s.index++
	return value, nil
}

func isSVGCommand(value byte) bool {
	return strings.ContainsRune("MmLlHhVvCcSsQqTtAaZz", rune(value))
}

func parseSVGPathData(source string) (parsedSVGPath, error) {
	scanner := svgPathScanner{source: source}
	path := parsedSVGPath{}
	var command byte
	var previousCommand byte
	var current svgPoint
	var subpathStart svgPoint
	var cubicControl svgPoint
	var quadraticControl svgPoint

	appendSegment := func(segment svgSegment) error {
		if len(path.subpaths) == 0 {
			return errors.New("drawing command before moveto")
		}
		last := len(path.subpaths) - 1
		path.subpaths[last].segments = append(path.subpaths[last].segments, segment)
		current = segmentEnd(segment)
		return nil
	}
	coordinate := func(relative bool) (svgPoint, error) {
		x, err := scanner.number()
		if err != nil {
			return svgPoint{}, err
		}
		y, err := scanner.number()
		if err != nil {
			return svgPoint{}, err
		}
		point := svgPoint{X: x, Y: y}
		if relative {
			point.X += current.X
			point.Y += current.Y
		}
		return point, nil
	}

	for {
		scanner.skipSeparators()
		if scanner.index >= len(source) {
			break
		}
		if isSVGCommand(source[scanner.index]) {
			command = source[scanner.index]
			scanner.index++
		} else if command == 0 {
			return parsedSVGPath{}, fmt.Errorf("expected command at byte %d", scanner.index)
		}

		relative := command >= 'a' && command <= 'z'
		upper := command
		if relative {
			upper -= 'a' - 'A'
		}
		if upper != 'Z' && !scanner.hasNumber() {
			return parsedSVGPath{}, fmt.Errorf("command %c missing parameters at byte %d", command, scanner.index)
		}

		switch upper {
		case 'M':
			point, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			current, subpathStart = point, point
			path.subpaths = append(path.subpaths, svgSubpath{start: point})
			previousCommand = 'M'
			if relative {
				command = 'l'
			} else {
				command = 'L'
			}
		case 'L':
			point, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			if err = appendSegment(svgSegment{command: 'L', points: [3]svgPoint{point}}); err != nil {
				return parsedSVGPath{}, err
			}
			previousCommand = 'L'
		case 'H':
			value, err := scanner.number()
			if err != nil {
				return parsedSVGPath{}, err
			}
			if relative {
				value += current.X
			}
			if err = appendSegment(svgSegment{command: 'L', points: [3]svgPoint{{X: value, Y: current.Y}}}); err != nil {
				return parsedSVGPath{}, err
			}
			previousCommand = 'H'
		case 'V':
			value, err := scanner.number()
			if err != nil {
				return parsedSVGPath{}, err
			}
			if relative {
				value += current.Y
			}
			if err = appendSegment(svgSegment{command: 'L', points: [3]svgPoint{{X: current.X, Y: value}}}); err != nil {
				return parsedSVGPath{}, err
			}
			previousCommand = 'V'
		case 'C':
			control1, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			control2, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			point, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			if err = appendSegment(svgSegment{command: 'C', points: [3]svgPoint{control1, control2, point}}); err != nil {
				return parsedSVGPath{}, err
			}
			cubicControl = control2
			previousCommand = 'C'
		case 'S':
			control1 := current
			if previousCommand == 'C' || previousCommand == 'S' {
				control1 = svgPoint{X: 2*current.X - cubicControl.X, Y: 2*current.Y - cubicControl.Y}
			}
			control2, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			point, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			if err = appendSegment(svgSegment{command: 'C', points: [3]svgPoint{control1, control2, point}}); err != nil {
				return parsedSVGPath{}, err
			}
			cubicControl = control2
			previousCommand = 'S'
		case 'Q':
			control, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			point, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			if err = appendSegment(svgSegment{command: 'Q', points: [3]svgPoint{control, point}}); err != nil {
				return parsedSVGPath{}, err
			}
			quadraticControl = control
			previousCommand = 'Q'
		case 'T':
			control := current
			if previousCommand == 'Q' || previousCommand == 'T' {
				control = svgPoint{X: 2*current.X - quadraticControl.X, Y: 2*current.Y - quadraticControl.Y}
			}
			point, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			if err = appendSegment(svgSegment{command: 'Q', points: [3]svgPoint{control, point}}); err != nil {
				return parsedSVGPath{}, err
			}
			quadraticControl = control
			previousCommand = 'T'
		case 'A':
			rx, err := scanner.number()
			if err != nil {
				return parsedSVGPath{}, err
			}
			ry, err := scanner.number()
			if err != nil {
				return parsedSVGPath{}, err
			}
			rotation, err := scanner.number()
			if err != nil {
				return parsedSVGPath{}, err
			}
			largeArc, err := scanner.flag()
			if err != nil {
				return parsedSVGPath{}, err
			}
			sweep, err := scanner.flag()
			if err != nil {
				return parsedSVGPath{}, err
			}
			point, err := coordinate(relative)
			if err != nil {
				return parsedSVGPath{}, err
			}
			segments := arcToCubic(current, point, rx, ry, rotation, largeArc, sweep)
			for _, segment := range segments {
				if err = appendSegment(segment); err != nil {
					return parsedSVGPath{}, err
				}
			}
			current = point
			previousCommand = 'A'
		case 'Z':
			if len(path.subpaths) == 0 {
				return parsedSVGPath{}, errors.New("closepath before moveto")
			}
			path.subpaths[len(path.subpaths)-1].closed = true
			current = subpathStart
			previousCommand = 'Z'
			command = 0
		default:
			return parsedSVGPath{}, fmt.Errorf("unsupported SVG command %q", command)
		}
	}
	if len(path.subpaths) == 0 {
		return parsedSVGPath{}, errors.New("path contains no subpaths")
	}
	return path, nil
}

func arcToCubic(start, end svgPoint, radiusX, radiusY, rotation float64, largeArc, sweep bool) []svgSegment {
	radiusX = math.Abs(radiusX)
	radiusY = math.Abs(radiusY)
	if radiusX == 0 || radiusY == 0 || start == end {
		if start == end {
			return nil
		}
		return []svgSegment{{command: 'L', points: [3]svgPoint{end}}}
	}
	angle := rotation * math.Pi / 180
	cosAngle, sinAngle := math.Cos(angle), math.Sin(angle)
	dx, dy := (start.X-end.X)/2, (start.Y-end.Y)/2
	xPrime := cosAngle*dx + sinAngle*dy
	yPrime := -sinAngle*dx + cosAngle*dy
	lambda := xPrime*xPrime/(radiusX*radiusX) + yPrime*yPrime/(radiusY*radiusY)
	if lambda > 1 {
		scale := math.Sqrt(lambda)
		radiusX *= scale
		radiusY *= scale
	}
	numerator := radiusX*radiusX*radiusY*radiusY - radiusX*radiusX*yPrime*yPrime - radiusY*radiusY*xPrime*xPrime
	denominator := radiusX*radiusX*yPrime*yPrime + radiusY*radiusY*xPrime*xPrime
	factor := 0.0
	if denominator > 0 {
		factor = math.Sqrt(max(0, numerator/denominator))
	}
	if largeArc == sweep {
		factor = -factor
	}
	cxPrime := factor * radiusX * yPrime / radiusY
	cyPrime := -factor * radiusY * xPrime / radiusX
	center := svgPoint{
		X: cosAngle*cxPrime - sinAngle*cyPrime + (start.X+end.X)/2,
		Y: sinAngle*cxPrime + cosAngle*cyPrime + (start.Y+end.Y)/2,
	}
	unitStart := svgPoint{X: (xPrime - cxPrime) / radiusX, Y: (yPrime - cyPrime) / radiusY}
	unitEnd := svgPoint{X: (-xPrime - cxPrime) / radiusX, Y: (-yPrime - cyPrime) / radiusY}
	startAngle := math.Atan2(unitStart.Y, unitStart.X)
	delta := vectorAngle(unitStart, unitEnd)
	if !sweep && delta > 0 {
		delta -= 2 * math.Pi
	} else if sweep && delta < 0 {
		delta += 2 * math.Pi
	}
	count := max(1, int(math.Ceil(math.Abs(delta)/(math.Pi/2))))
	step := delta / float64(count)
	segments := make([]svgSegment, 0, count)
	for index := 0; index < count; index++ {
		from := startAngle + float64(index)*step
		to := from + step
		alpha := 4.0 / 3.0 * math.Tan((to-from)/4)
		fromPoint := ellipsePoint(center, radiusX, radiusY, cosAngle, sinAngle, from)
		toPoint := ellipsePoint(center, radiusX, radiusY, cosAngle, sinAngle, to)
		fromDerivative := ellipseDerivative(radiusX, radiusY, cosAngle, sinAngle, from)
		toDerivative := ellipseDerivative(radiusX, radiusY, cosAngle, sinAngle, to)
		control1 := svgPoint{X: fromPoint.X + alpha*fromDerivative.X, Y: fromPoint.Y + alpha*fromDerivative.Y}
		control2 := svgPoint{X: toPoint.X - alpha*toDerivative.X, Y: toPoint.Y - alpha*toDerivative.Y}
		segments = append(segments, svgSegment{command: 'C', points: [3]svgPoint{control1, control2, toPoint}})
	}
	segments[len(segments)-1].points[2] = end
	return segments
}

func vectorAngle(left, right svgPoint) float64 {
	return math.Atan2(left.X*right.Y-left.Y*right.X, left.X*right.X+left.Y*right.Y)
}

func ellipsePoint(center svgPoint, radiusX, radiusY, cosAngle, sinAngle, angle float64) svgPoint {
	cosValue, sinValue := math.Cos(angle), math.Sin(angle)
	return svgPoint{
		X: center.X + radiusX*cosAngle*cosValue - radiusY*sinAngle*sinValue,
		Y: center.Y + radiusX*sinAngle*cosValue + radiusY*cosAngle*sinValue,
	}
}

func ellipseDerivative(radiusX, radiusY, cosAngle, sinAngle, angle float64) svgPoint {
	cosValue, sinValue := math.Cos(angle), math.Sin(angle)
	return svgPoint{
		X: -radiusX*cosAngle*sinValue - radiusY*sinAngle*cosValue,
		Y: -radiusX*sinAngle*sinValue + radiusY*cosAngle*cosValue,
	}
}

type svgViewBox struct {
	MinX   float64
	MinY   float64
	Width  float64
	Height float64
}

func renderSVGPathMask(path parsedSVGPath, viewBox svgViewBox, size int, evenOdd bool) *image.Alpha {
	if evenOdd && len(path.subpaths) > 1 {
		result := image.NewAlpha(image.Rect(0, 0, size, size))
		for _, subpath := range path.subpaths {
			mask := renderSVGSubpaths([]svgSubpath{subpath}, viewBox, size)
			for index, value := range mask.Pix {
				previous := int(result.Pix[index])
				next := int(value)
				result.Pix[index] = uint8((previous*(255-next) + next*(255-previous) + 127) / 255)
			}
		}
		return result
	}
	return renderSVGSubpaths(path.subpaths, viewBox, size)
}

func renderSVGSubpaths(subpaths []svgSubpath, viewBox svgViewBox, size int) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, size, size))
	raster := vector.NewRasterizer(size, size)
	margin := float32(size) * 0.05
	scaleX := (float32(size) - 2*margin) / float32(viewBox.Width)
	scaleY := (float32(size) - 2*margin) / float32(viewBox.Height)
	transform := func(point svgPoint) (float32, float32) {
		return margin + float32(point.X-viewBox.MinX)*scaleX, margin + float32(point.Y-viewBox.MinY)*scaleY
	}
	for _, subpath := range subpaths {
		x, y := transform(subpath.start)
		raster.MoveTo(x, y)
		for _, segment := range subpath.segments {
			switch segment.command {
			case 'L':
				x, y = transform(segment.points[0])
				raster.LineTo(x, y)
			case 'Q':
				x1, y1 := transform(segment.points[0])
				x2, y2 := transform(segment.points[1])
				raster.QuadTo(x1, y1, x2, y2)
			case 'C':
				x1, y1 := transform(segment.points[0])
				x2, y2 := transform(segment.points[1])
				x3, y3 := transform(segment.points[2])
				raster.CubeTo(x1, y1, x2, y2, x3, y3)
			}
		}
		if subpath.closed {
			raster.ClosePath()
		}
	}
	raster.Draw(mask, mask.Bounds(), image.NewUniform(color.Alpha{A: 0xFF}), image.Point{})
	return mask
}
