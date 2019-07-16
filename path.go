package canvas

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/tdewolff/parse/strconv"
	"golang.org/x/image/vector"
)

// Tolerance is the maximum deviation from the original path in millimeters when e.g. flatting
var Tolerance = 0.01

// FillRule defines the FillRuleType used by the path.
var FillRule = NonZero

// FillRuleType is the algorithm to specify which area is to be filled and which not, in particular when multiple path segments overlap. The NonZero rule is the default and will fill any point that is being enclosed by an unequal number of paths winding clockwise and counter clockwise, otherwise it will not be filled. The EvenOdd rule will fill any point that is being enclosed by an uneven number of path, whichever their direction.
type FillRuleType int

const (
	NonZero FillRuleType = iota // see FillRuleType
	EvenOdd
)

const (
	moveToCmd = 1.0 << iota
	LineToCmd
	QuadToCmd
	CubeToCmd
	ArcToCmd
	CloseCmd
	NullCmd = 0.0
)

// cmdLen returns the number of numbers the path command contains.
func cmdLen(cmd float64) int {
	switch cmd {
	case moveToCmd, LineToCmd, CloseCmd:
		return 3
	case QuadToCmd:
		return 5
	case CubeToCmd, ArcToCmd:
		return 7
	case NullCmd:
		return 0
	}
	fmt.Println("unknown", cmd)
	panic("unknown path command")
}

func fromArcFlags(f float64) (bool, bool) {
	largeArc := (f == 1.0 || f == 3.0)
	sweep := (f == 2.0 || f == 3.0)
	return largeArc, sweep
}

func toArcFlags(largeArc, sweep bool) float64 {
	f := 0.0
	if largeArc {
		f += 1.0
	}
	if sweep {
		f += 2.0
	}
	return f
}

// Path defines a vector path in 2D using a series of connected commands (MoveTo, LineTo, QuadTo, CubeTo, ArcTo and Close).
// Each command consists of a number of float64 values (depending on the command) that fully define the action. The first value is the command itself (as a float64). The last two values are the end point position of the pen after the action (x,y). QuadTo defined one control point (x,y) in between, CubeTo defines two control points, and ArcTo defines (rx,ry,phi,largeArc+sweep) i.e. the radius in x and y, its rotation (in radians) and the largeArc and sweep booleans in one float64.
// Only valid commands are appended, so that LineTo has a non-zero length, QuadTo's and CubeTo's control point(s) don't (both) overlap with the start and end point, and ArcTo has non-zero radii and has non-zero length. For ArcTo we also make sure the angle is is in the range [0, 2*PI) and we scale the radii up if they appear too small to fit the arc.
type Path struct {
	d  []float64
	i0 int // index of last MoveTo command
}

// Empty returns true if p is an empty path or consists of only MoveTos and Closes.
func (p *Path) Empty() bool {
	if len(p.d) == 0 {
		return true
	}
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		if cmd != moveToCmd && cmd != CloseCmd {
			return false
		}
		i += cmdLen(cmd)
	}
	return true
}

// Closed returns true if the last segment of p is a closed path.
func (p *Path) Closed() bool {
	var cmd float64
	for i := p.i0; i < len(p.d); {
		cmd = p.d[i]
		i += cmdLen(cmd)
	}
	return cmd == CloseCmd
}

// Copy returns a copy of p.
func (p *Path) Copy() *Path {
	q := &Path{}
	q.d = append(q.d, p.d...)
	q.i0 = p.i0
	return q
}

// Append appends path q to p and returns a new path.
func (p *Path) Append(q *Path) *Path {
	if q == nil || len(q.d) == 0 {
		return p
	} else if len(p.d) == 0 {
		return q
	}
	i0 := len(p.d) + q.i0
	if q.d[0] != moveToCmd {
		p.MoveTo(0.0, 0.0)
		if q.i0 != 0 {
			i0 += cmdLen(moveToCmd)
		}
	}
	return &Path{append(p.d, q.d...), i0}
}

// Join joins path q to p and returns a new path.
func (p *Path) Join(q *Path) *Path {
	if q == nil || len(q.d) == 0 {
		return p
	} else if len(p.d) == 0 {
		return q
	}
	i0 := len(p.d) + q.i0
	if q.d[0] == moveToCmd {
		x0, y0 := p.d[len(p.d)-2], p.d[len(p.d)-1]
		x1, y1 := q.d[1], q.d[2]
		if equal(x0, x1) && equal(y0, y1) {
			q.d = q.d[3:]
			i0 -= 3
			if q.i0 == 0 {
				i0 = p.i0
			}
		}
	} else if q.i0 == 0 {
		i0 = p.i0
	}
	return &Path{append(p.d, q.d...), i0}
}

// Pos returns the current position of the path, which is the end point of the last command.
func (p *Path) Pos() Point {
	if 0 < len(p.d) {
		return Point{p.d[len(p.d)-2], p.d[len(p.d)-1]}
	}
	return Point{}
}

// StartPos returns the start point of the current path segment, ie. it returns the position of the last MoveTo command.
func (p *Path) StartPos() Point {
	if len(p.d) > 0 && p.d[p.i0] == moveToCmd {
		return Point{p.d[p.i0+1], p.d[p.i0+2]}
	}
	return Point{}
}

// Coords returns all the coordinates between the commands.
func (p *Path) Coords() []Point {
	P := []Point{}
	if 0 < len(p.d) && p.d[0] != moveToCmd {
		P = append(P, Point{0.0, 0.0})
	}
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		i += cmdLen(cmd)
		P = append(P, Point{p.d[i-2], p.d[i-1]})
	}
	return P
}

////////////////////////////////////////////////////////////////

// MoveTo moves the path to x,y without connecting the path. It starts a new independent path segment.
// Multiple path segments can be useful when negating parts of a previous path by overlapping it
// with a path in the opposite direction.
func (p *Path) MoveTo(x, y float64) *Path {
	p.i0 = len(p.d)
	p.d = append(p.d, moveToCmd, x, y)
	return p
}

// LineTo adds a linear path to x1,y1.
func (p *Path) LineTo(x1, y1 float64) *Path {
	p0 := p.Pos()
	p1 := Point{x1, y1}
	if p0.Equals(p1) {
		return p
	}
	p.d = append(p.d, LineToCmd, p1.X, p1.Y)
	return p
}

// QuadTo adds a quadratic Bézier path with control point cpx,cpy and end point x1,y1.
func (p *Path) QuadTo(cpx, cpy, x1, y1 float64) *Path {
	p0 := p.Pos()
	cp := Point{cpx, cpy}
	p1 := Point{x1, y1}
	if cp.Equals(p0) || cp.Equals(p1) {
		return p.LineTo(p1.X, p1.Y)
	}
	p.d = append(p.d, QuadToCmd, cp.X, cp.Y, p1.X, p1.Y)
	return p
}

// CubeTo adds a cubic Bézier path with control points cpx1,cpy1 and cpx2,cpy2 and end point x1,y1.
func (p *Path) CubeTo(cpx1, cpy1, cpx2, cpy2, x1, y1 float64) *Path {
	p0 := p.Pos()
	cp1 := Point{cpx1, cpy1}
	cp2 := Point{cpx2, cpy2}
	p1 := Point{x1, y1}
	if (cp1.Equals(p0) || cp1.Equals(p1)) && (cp2.Equals(p0) || cp2.Equals(p1)) {
		return p.LineTo(p1.X, p1.Y)
	}
	p.d = append(p.d, CubeToCmd, cp1.X, cp1.Y, cp2.X, cp2.Y, p1.X, p1.Y)
	return p
}

// ArcTo adds an arc with radii rx and ry, with rot the counter clockwise rotation with respect to the coordinate system in degrees,
// largeArc and sweep booleans (see https://developer.mozilla.org/en-US/docs/Web/SVG/Tutorial/Paths#Arcs),
// and x1,y1 the end position of the pen. The start positions of the pen was given by a previous command.
// When sweep is true it means following the arc in a CCW direction in the Cartesian coordinate system, ie. that is CW in the upper-left coordinate system as is the case in SVGs.
func (p *Path) ArcTo(rx, ry, rot float64, largeArc, sweep bool, x1, y1 float64) *Path {
	p0 := p.Pos()
	p1 := Point{x1, y1}
	if p0.Equals(p1) {
		return p
	}
	if equal(rx, 0.0) || equal(ry, 0.0) {
		return p.LineTo(p1.X, p1.Y)
	}

	rx = math.Abs(rx)
	ry = math.Abs(ry)
	if rx < ry {
		rx, ry = ry, rx
		rot += 90.0
	}

	phi := angleNorm(rot * math.Pi / 180.0)
	if math.Pi <= phi { // phi is canonical within 0 <= phi < 180
		phi -= math.Pi
	}

	// scale ellipse if rx and ry are too small
	lambda := ellipseRadiiCorrection(p0, rx, ry, phi, p1)
	if lambda > 1.0 {
		rx *= lambda
		ry *= lambda
	}

	p.d = append(p.d, ArcToCmd, rx, ry, phi, toArcFlags(largeArc, sweep), p1.X, p1.Y)
	return p
}

// Arc adds an elliptical arc with radii rx and ry, with rot the counter clockwise rotation in degrees, and theta0 and theta1 the angles in degrees of the ellipse (before rot is applies) between which the arc will run. If theta0 < theta1, the arc will run in a CCW direction. If the difference between theta0 and theta1 is bigger than 360 degrees, a full circle will be drawn and the remaining part (eg. a difference of 810 degrees will draw one full circle and an arc over 90 degrees).
func (p *Path) Arc(rx, ry, rot, theta0, theta1 float64) *Path {
	phi := rot * math.Pi / 180.0
	theta0 *= math.Pi / 180.0
	theta1 *= math.Pi / 180.0
	dtheta := math.Abs(theta1 - theta0)

	sweep := theta0 < theta1
	largeArc := math.Mod(dtheta, 2.0*math.Pi) > math.Pi
	p0 := ellipsePos(rx, ry, phi, 0.0, 0.0, theta0)
	p1 := ellipsePos(rx, ry, phi, 0.0, 0.0, theta1)

	start := p.Pos()
	center := start.Sub(p0)
	if dtheta > 2.0*math.Pi {
		startOpposite := center.Sub(p0)
		p.ArcTo(rx, ry, rot, largeArc, sweep, startOpposite.X, startOpposite.Y)
		p.ArcTo(rx, ry, rot, largeArc, sweep, start.X, start.Y)
	}
	end := center.Add(p1)
	return p.ArcTo(rx, ry, rot, largeArc, sweep, end.X, end.Y)
}

// Close closes a path with a LineTo to the start of the path (the most recent MoveTo command).
// It also signals the path closes, as opposed to being just a LineTo command.
func (p *Path) Close() *Path {
	start := p.StartPos()
	p.d = append(p.d, CloseCmd, start.X, start.Y)
	return p
}

////////////////////////////////////////////////////////////////

// CCW returns true when the path has (mostly) a counter clockwise direction.
func (p *Path) CCW() bool {
	// use the Shoelace formula
	area := 0.0
	var start, end Point
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		i += cmdLen(cmd)

		end = Point{p.d[i-2], p.d[i-1]}
		if cmd != moveToCmd {
			area += (end.X - start.X) * (start.Y + end.Y)
		}
		start = end
	}
	return area < 0.0
}

// Filling returns whether the path segments get filled or not. A path may not be filling when it negates another path, depending on the FillRule.
func (p *Path) Filling() []bool {
	Ps := p.Split()
	testPoints := make([]Point, 0, len(Ps))
	for _, ps := range Ps {
		if !ps.Closed() || ps.Empty() {
			continue
		}

		var p0, p1 Point
		iNextCmd := cmdLen(ps.d[0])
		if ps.d[0] != moveToCmd {
			p1 = Point{ps.d[iNextCmd-2], ps.d[iNextCmd-1]}
		} else {
			iNextCmd2 := iNextCmd + cmdLen(ps.d[iNextCmd])
			p0 = Point{ps.d[iNextCmd-2], ps.d[iNextCmd-1]}
			p1 = Point{ps.d[iNextCmd2-2], ps.d[iNextCmd2-1]}
		}

		offset := p1.Sub(p0).Rot90CW().Norm(Epsilon)
		if ps.CCW() {
			offset = offset.Neg()
		}
		testPoints = append(testPoints, p0.Interpolate(p1, 0.5).Add(offset))
	}

	fillCounts := make([]int, len(testPoints))
	for _, ps := range Ps {
		polyline := PolylineFromPathCoords(ps) // no need for flattening as we pick our test point to be inside the polyline
		for i, test := range testPoints {
			fillCounts[i] += polyline.FillCount(test.X, test.Y)
		}
	}

	fillings := make([]bool, len(fillCounts))
	for i := range fillCounts {
		if FillRule == NonZero {
			fillings[i] = fillCounts[i] != 0
		} else {
			fillings[i] = fillCounts[i]%2 != 0
		}
	}
	return fillings
}

// Interior is true when the point (x,y) is in the interior of the path, ie. gets filled. This depends on the FillRule.
func (p *Path) Interior(x, y float64) bool {
	fillCount := 0
	test := Point{x, y}
	for _, ps := range p.Split() {
		fillCount += PolylineFromPath(ps).FillCount(test.X, test.Y) // uses flattening
	}
	if FillRule == NonZero {
		return fillCount != 0
	}
	return fillCount%2 != 0
}

// Bounds returns the bounding box rectangle of the path.
func (p *Path) Bounds() Rect {
	if len(p.d) == 0 {
		return Rect{}
	}

	xmin, xmax := math.Inf(1), math.Inf(-1)
	ymin, ymax := math.Inf(1), math.Inf(-1)
	if len(p.d) > 0 && p.d[0] != moveToCmd {
		xmin = 0.0
		xmax = 0.0
		ymin = 0.0
		ymax = 0.0
	}

	var start, end Point
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		switch cmd {
		case moveToCmd:
			end = Point{p.d[i+1], p.d[i+2]}
			xmin = math.Min(xmin, end.X)
			xmax = math.Max(xmax, end.X)
			ymin = math.Min(ymin, end.Y)
			ymax = math.Max(ymax, end.Y)
		case LineToCmd, CloseCmd:
			end = Point{p.d[i+1], p.d[i+2]}
			xmin = math.Min(xmin, end.X)
			xmax = math.Max(xmax, end.X)
			ymin = math.Min(ymin, end.Y)
			ymax = math.Max(ymax, end.Y)
		case QuadToCmd:
			cp := Point{p.d[i+1], p.d[i+2]}
			end = Point{p.d[i+3], p.d[i+4]}

			xmin = math.Min(xmin, end.X)
			xmax = math.Max(xmax, end.X)
			if tdenom := (start.X - 2*cp.X + end.X); tdenom != 0.0 {
				if t := (start.X - cp.X) / tdenom; 0.0 < t && t < 1.0 {
					x := quadraticBezierPos(start, cp, end, t)
					xmin = math.Min(xmin, x.X)
					xmax = math.Max(xmax, x.X)
				}
			}

			ymin = math.Min(ymin, end.Y)
			ymax = math.Max(ymax, end.Y)
			if tdenom := (start.Y - 2*cp.Y + end.Y); tdenom != 0.0 {
				if t := (start.Y - cp.Y) / tdenom; 0.0 < t && t < 1.0 {
					y := quadraticBezierPos(start, cp, end, t)
					ymin = math.Min(ymin, y.Y)
					ymax = math.Max(ymax, y.Y)
				}
			}
		case CubeToCmd:
			cp1 := Point{p.d[i+1], p.d[i+2]}
			cp2 := Point{p.d[i+3], p.d[i+4]}
			end = Point{p.d[i+5], p.d[i+6]}

			a := -start.X + 3*cp1.X - 3*cp2.X + end.X
			b := 2*start.X - 4*cp1.X + 2*cp2.X
			c := -start.X + cp1.X
			t1, t2 := solveQuadraticFormula(a, b, c)

			xmin = math.Min(xmin, end.X)
			xmax = math.Max(xmax, end.X)
			if !math.IsNaN(t1) && 0.0 < t1 && t1 < 1.0 {
				x1 := cubicBezierPos(start, cp1, cp2, end, t1)
				xmin = math.Min(xmin, x1.X)
				xmax = math.Max(xmax, x1.X)
			}
			if !math.IsNaN(t2) && 0.0 < t2 && t2 < 1.0 {
				x2 := cubicBezierPos(start, cp1, cp2, end, t2)
				xmin = math.Min(xmin, x2.X)
				xmax = math.Max(xmax, x2.X)
			}

			a = -start.Y + 3*cp1.Y - 3*cp2.Y + end.Y
			b = 2*start.Y - 4*cp1.Y + 2*cp2.Y
			c = -start.Y + cp1.Y
			t1, t2 = solveQuadraticFormula(a, b, c)

			ymin = math.Min(ymin, end.Y)
			ymax = math.Max(ymax, end.Y)
			if !math.IsNaN(t1) && 0.0 < t1 && t1 < 1.0 {
				y1 := cubicBezierPos(start, cp1, cp2, end, t1)
				ymin = math.Min(ymin, y1.Y)
				ymax = math.Max(ymax, y1.Y)
			}
			if !math.IsNaN(t2) && 0.0 < t2 && t2 < 1.0 {
				y2 := cubicBezierPos(start, cp1, cp2, end, t2)
				ymin = math.Min(ymin, y2.Y)
				ymax = math.Max(ymax, y2.Y)
			}
		case ArcToCmd:
			rx, ry, phi := p.d[i+1], p.d[i+2], p.d[i+3]
			largeArc, sweep := fromArcFlags(p.d[i+4])
			end = Point{p.d[i+5], p.d[i+6]}
			cx, cy, theta1, theta2 := ellipseToCenter(start.X, start.Y, rx, ry, phi, largeArc, sweep, end.X, end.Y)

			// find the four extremes (top, bottom, left, right) and apply those who are between theta1 and theta2
			// x(theta) = cx + rx*cos(theta)*cos(phi) - ry*sin(theta)*sin(phi)
			// y(theta) = cy + rx*cos(theta)*sin(phi) + ry*sin(theta)*cos(phi)
			// be aware that positive rotation appears clockwise in SVGs (non-Cartesian coordinate system)
			// we can now find the angles of the extremes

			sinphi, cosphi := math.Sincos(phi)
			thetaRight := math.Atan2(-ry*sinphi, rx*cosphi)
			thetaTop := math.Atan2(rx*cosphi, ry*sinphi)
			thetaLeft := thetaRight + math.Pi
			thetaBottom := thetaTop + math.Pi

			dx := math.Sqrt(rx*rx*cosphi*cosphi + ry*ry*sinphi*sinphi)
			dy := math.Sqrt(rx*rx*sinphi*sinphi + ry*ry*cosphi*cosphi)
			if angleBetween(thetaLeft, theta1, theta2) {
				xmin = math.Min(xmin, cx-dx)
			}
			if angleBetween(thetaRight, theta1, theta2) {
				xmax = math.Max(xmax, cx+dx)
			}
			if angleBetween(thetaBottom, theta1, theta2) {
				ymin = math.Min(ymin, cy-dy)
			}
			if angleBetween(thetaTop, theta1, theta2) {
				ymax = math.Max(ymax, cy+dy)
			}
			xmin = math.Min(xmin, end.X)
			xmax = math.Max(xmax, end.X)
			ymin = math.Min(ymin, end.Y)
			ymax = math.Max(ymax, end.Y)
		}
		i += cmdLen(cmd)
		start = end
	}
	return Rect{xmin, ymin, xmax - xmin, ymax - ymin}
}

// Length returns the length of the path in millimeters. The length is approximated for cubic Béziers.
func (p *Path) Length() float64 {
	d := 0.0
	var start, end Point
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		switch cmd {
		case moveToCmd:
			end = Point{p.d[i+1], p.d[i+2]}
		case LineToCmd, CloseCmd:
			end = Point{p.d[i+1], p.d[i+2]}
			d += end.Sub(start).Length()
		case QuadToCmd:
			cp := Point{p.d[i+1], p.d[i+2]}
			end = Point{p.d[i+3], p.d[i+4]}
			d += quadraticBezierLength(start, cp, end)
		case CubeToCmd:
			cp1 := Point{p.d[i+1], p.d[i+2]}
			cp2 := Point{p.d[i+3], p.d[i+4]}
			end = Point{p.d[i+5], p.d[i+6]}
			d += cubicBezierLength(start, cp1, cp2, end)
		case ArcToCmd:
			rx, ry, phi := p.d[i+1], p.d[i+2], p.d[i+3]
			largeArc, sweep := fromArcFlags(p.d[i+4])
			end = Point{p.d[i+5], p.d[i+6]}
			_, _, theta1, theta2 := ellipseToCenter(start.X, start.Y, rx, ry, phi, largeArc, sweep, end.X, end.Y)
			d += ellipseLength(rx, ry, theta1, theta2)
		}
		i += cmdLen(cmd)
		start = end
	}
	return d
}

// Transform transform the path by the given transformation matrix and returns a new path.
func (p *Path) Transform(m Matrix) *Path {
	p = p.Copy()
	if len(p.d) > 0 && p.d[0] != moveToCmd {
		p.d = append([]float64{moveToCmd, 0.0, 0.0}, p.d...)
	}
	_, _, _, xscale, yscale, _ := m.Decompose()
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		switch cmd {
		case moveToCmd, LineToCmd, CloseCmd:
			end := m.Dot(Point{p.d[i+1], p.d[i+2]})
			p.d[i+1] = end.X
			p.d[i+2] = end.Y
		case QuadToCmd:
			cp := m.Dot(Point{p.d[i+1], p.d[i+2]})
			end := m.Dot(Point{p.d[i+3], p.d[i+4]})
			p.d[i+1] = cp.X
			p.d[i+2] = cp.Y
			p.d[i+3] = end.X
			p.d[i+4] = end.Y
		case CubeToCmd:
			cp1 := m.Dot(Point{p.d[i+1], p.d[i+2]})
			cp2 := m.Dot(Point{p.d[i+3], p.d[i+4]})
			end := m.Dot(Point{p.d[i+5], p.d[i+6]})
			p.d[i+1] = cp1.X
			p.d[i+2] = cp1.Y
			p.d[i+3] = cp2.X
			p.d[i+4] = cp2.Y
			p.d[i+5] = end.X
			p.d[i+6] = end.Y
		case ArcToCmd:
			rx := p.d[i+1]
			ry := p.d[i+2]
			phi := p.d[i+3]
			largeArc, sweep := fromArcFlags(p.d[i+4])
			end := m.Dot(Point{p.d[i+5], p.d[i+6]})

			// For ellipses written as the conic section equation in matrix form, we have:
			// (x, y) E (x; y) = 0, with E = (1/rx^2, 0; 0, 1/ry^2)
			// for our transformed ellipse we have (x', y') = T (x, y), with T the affine transformation matrix
			// so that (T^-1 (x'; y'))^T E (T^-1 (x'; y') = 0  =>  (x', y') T^(-1,T) E T^(-1) (x'; y') = 0
			// we define Q = T^(-1,T) E T^(-1) the new ellipse equation which is typically rotated from the x-axis.
			// That's why we find the eigenvalues and eigenvectors (the new direction and length of the major and minor axes).
			T := m.Rotate(phi * 180.0 / math.Pi)
			invT := T.Inv()
			Q := Identity.Scale(1.0/rx/rx, 1.0/ry/ry)
			Q = invT.T().Mul(Q).Mul(invT)

			lambda1, lambda2, v1, v2 := Q.Eigen()
			rx = 1 / math.Sqrt(lambda1)
			ry = 1 / math.Sqrt(lambda2)
			phi = v1.Angle()
			if rx < ry {
				rx, ry = ry, rx
				phi = v2.Angle()
			}
			phi = angleNorm(phi)
			if math.Pi <= phi { // phi is canonical within 0 <= phi < 180
				phi -= math.Pi
			}

			if xscale*yscale < 0.0 { // flip x or y axis needs flipping of the sweep
				sweep = !sweep
			}
			p.d[i+1] = rx
			p.d[i+2] = ry
			p.d[i+3] = phi
			p.d[i+4] = toArcFlags(largeArc, sweep)
			p.d[i+5] = end.X
			p.d[i+6] = end.Y
		}
		i += cmdLen(cmd)
	}
	return p
}

// Translate translates the path by (x,y) and returns a new path.
func (p *Path) Translate(x, y float64) *Path {
	return p.Transform(Identity.Translate(x, y))
}

// Flatten flattens all Bézier and arc curves into linear segments and returns a new path. It uses Tolerance as the maximum deviation.
func (p *Path) Flatten() *Path {
	return p.Copy().Replace(nil, flattenCubicBezier, flattenEllipse)
}

// Replace replaces path commands by their respective functions, each returning the path that will replace the command or nil if no replacement is to be performed. The line function will take the start and end points. The bezier function will take the start point, control point 1 and 2, and the end point (ie. a cubic Bézier, quadratic Béziers will be implicitly converted to cubic ones). The arc function will take a start point, the major and minor radii, the radial rotaton counter clockwise, the largeArc and sweep booleans, and the end point. Be aware this will change the path inplace.
func (p *Path) Replace(
	line func(Point, Point) *Path,
	bezier func(Point, Point, Point, Point) *Path,
	arc func(Point, float64, float64, float64, bool, bool, Point) *Path,
) *Path {
	start := Point{}
	for i := 0; i < len(p.d); {
		var q *Path
		cmd := p.d[i]
		switch cmd {
		case moveToCmd:
			p.i0 = i
		case LineToCmd, CloseCmd:
			if line != nil {
				end := Point{p.d[i+1], p.d[i+2]}
				q = line(start, end)
				if cmd == CloseCmd {
					q.Close()
				}
			}
		case QuadToCmd:
			if bezier != nil {
				cp := Point{p.d[i+1], p.d[i+2]}
				end := Point{p.d[i+3], p.d[i+4]}
				cp1, cp2 := quadraticToCubicBezier(start, cp, end)
				q = bezier(start, cp1, cp2, end)
			}
		case CubeToCmd:
			if bezier != nil {
				cp1 := Point{p.d[i+1], p.d[i+2]}
				cp2 := Point{p.d[i+3], p.d[i+4]}
				end := Point{p.d[i+5], p.d[i+6]}
				q = bezier(start, cp1, cp2, end)
			}
		case ArcToCmd:
			if arc != nil {
				rx, ry, phi := p.d[i+1], p.d[i+2], p.d[i+3]
				largeArc, sweep := fromArcFlags(p.d[i+4])
				end := Point{p.d[i+5], p.d[i+6]}
				q = arc(start, rx, ry, phi, largeArc, sweep, end)
			}
		}

		if q != nil {
			if 0 < len(q.d) && q.d[0] == moveToCmd {
				x0, y0 := 0.0, 0.0
				if 0 < i {
					x0, y0 = p.d[i-2], p.d[i-1]
				}
				x1, y1 := q.d[1], q.d[2]
				if equal(x0, x1) && equal(y0, y1) {
					q.d = q.d[3:]
				}
			}
			p.d = append(p.d[:i:i], append(q.d, p.d[i+cmdLen(cmd):]...)...)
			i += len(q.d)
			if q.Empty() {
				continue // don't update start variable
			}
		} else {
			i += cmdLen(cmd)
		}
		start = Point{p.d[i-2], p.d[i-1]}
	}
	return p
}

// Markers returns an array of start, mid and end markers along the path at the path coordinates between commands. Align will align the markers with the path direction.
func (p *Path) Markers(start, mid, end *Path, align bool) []*Path {
	coord := Point{}
	markers := []*Path{}
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		if len(markers) != 0 || cmd == moveToCmd {
			i += cmdLen(cmd)
			coord = Point{p.d[i-2], p.d[i-1]}
		}
		if cmd != moveToCmd {
			q := mid
			if len(markers) == 0 {
				q = start
			} else if i == len(p.d) {
				q = end
			}

			m := Identity.Translate(coord.X, coord.Y)
			if align {
				// TODO: marker alignment
			}
			markers = append(markers, q.Copy().Transform(m))
		}
	}
	return markers
}

// Split splits the path into its independent path segments. The path is split on the MoveTo and/or Close commands.
func (p *Path) Split() []*Path {
	ps := []*Path{}
	start := Point{}
	closed := false
	var i, j int
	for j < len(p.d) {
		cmd := p.d[j]
		if j > i && cmd == moveToCmd || closed {
			d := p.d[i:j:j]
			if d[0] != moveToCmd && !start.IsZero() {
				d = append([]float64{moveToCmd, start.X, start.Y}, d...)
			}
			ps = append(ps, &Path{d, 0})
			start = Point{p.d[j-2], p.d[j-1]}
			i = j
		}
		closed = cmd == CloseCmd
		j += cmdLen(cmd)
	}
	if j > i {
		d := p.d[i:j:j]
		if d[0] != moveToCmd && !start.IsZero() {
			d = append([]float64{moveToCmd, start.X, start.Y}, d...)
		}
		ps = append(ps, &Path{d, 0})
	}
	return ps
}

// SplitAt splits the path into separate paths at the specified intervals (given in millimeters) along the path.
func (p *Path) SplitAt(ts ...float64) []*Path {
	if len(ts) == 0 {
		return []*Path{}
	}

	sort.Float64s(ts)
	if ts[0] == 0.0 {
		ts = ts[1:]
	}

	j := 0   // index into ts
	T := 0.0 // current position along curve

	qs := []*Path{}
	q := &Path{}
	push := func() {
		qs = append(qs, q)
		q = &Path{}
	}

	if len(p.d) > 0 && p.d[0] == moveToCmd {
		q.MoveTo(p.d[1], p.d[2])
	}
	for _, ps := range p.Split() {
		var start, end Point
		for i := 0; i < len(ps.d); {
			cmd := ps.d[i]
			switch cmd {
			case moveToCmd:
				end = Point{p.d[i+1], p.d[i+2]}
			case LineToCmd, CloseCmd:
				end = Point{p.d[i+1], p.d[i+2]}

				if j == len(ts) {
					q.LineTo(end.X, end.Y)
				} else {
					dT := end.Sub(start).Length()
					Tcurve := T
					for j < len(ts) && T < ts[j] && ts[j] <= T+dT {
						tpos := (ts[j] - T) / dT
						pos := start.Interpolate(end, tpos)
						Tcurve = ts[j]

						q.LineTo(pos.X, pos.Y)
						push()
						q.MoveTo(pos.X, pos.Y)
						j++
					}
					if Tcurve < T+dT {
						q.LineTo(end.X, end.Y)
					}
					T += dT
				}
			case QuadToCmd:
				cp := Point{p.d[i+1], p.d[i+2]}
				end = Point{p.d[i+3], p.d[i+4]}

				if j == len(ts) {
					q.QuadTo(cp.X, cp.Y, end.X, end.Y)
				} else {
					speed := func(t float64) float64 {
						return quadraticBezierDeriv(start, cp, end, t).Length()
					}
					invL, dT := invSpeedPolynomialChebyshevApprox(10, gaussLegendre5, speed, 0.0, 1.0)

					t0 := 0.0
					r0, r1, r2 := start, cp, end
					for j < len(ts) && T < ts[j] && ts[j] <= T+dT {
						t := invL(ts[j] - T)
						tsub := (t - t0) / (1.0 - t0)
						t0 = t

						var q1 Point
						_, q1, _, r0, r1, r2 = splitQuadraticBezier(r0, r1, r2, tsub)

						q.QuadTo(q1.X, q1.Y, r0.X, r0.Y)
						push()
						q.MoveTo(r0.X, r0.Y)
						j++
					}
					if !equal(t0, 1.0) {
						q.QuadTo(r1.X, r1.Y, r2.X, r2.Y)
					}
					T += dT
				}
			case CubeToCmd:
				cp1 := Point{p.d[i+1], p.d[i+2]}
				cp2 := Point{p.d[i+3], p.d[i+4]}
				end = Point{p.d[i+5], p.d[i+6]}

				if j == len(ts) {
					q.CubeTo(cp1.X, cp1.Y, cp2.X, cp2.Y, end.X, end.Y)
				} else {
					// TODO: handle inflection points (not sure if really needed)
					speed := func(t float64) float64 {
						return cubicBezierDeriv(start, cp1, cp2, end, t).Length()
					}
					invL, dT := invSpeedPolynomialChebyshevApprox(10, gaussLegendre5, speed, 0.0, 1.0)

					t0 := 0.0
					r0, r1, r2, r3 := start, cp1, cp2, end
					for j < len(ts) && T < ts[j] && ts[j] <= T+dT {
						t := invL(ts[j] - T)
						tsub := (t - t0) / (1.0 - t0)
						t0 = t

						var q1, q2 Point
						_, q1, q2, _, r0, r1, r2, r3 = splitCubicBezier(r0, r1, r2, r3, tsub)

						q.CubeTo(q1.X, q1.Y, q2.X, q2.Y, r0.X, r0.Y)
						push()
						q.MoveTo(r0.X, r0.Y)
						j++
					}
					if !equal(t0, 1.0) {
						q.CubeTo(r1.X, r1.Y, r2.X, r2.Y, r3.X, r3.Y)
					}
					T += dT
				}
			case ArcToCmd:
				rx, ry, phi := p.d[i+1], p.d[i+2], p.d[i+3]
				largeArc, sweep := fromArcFlags(p.d[i+4])
				end = Point{p.d[i+5], p.d[i+6]}
				cx, cy, theta1, theta2 := ellipseToCenter(start.X, start.Y, rx, ry, phi, largeArc, sweep, end.X, end.Y)

				if j == len(ts) {
					q.ArcTo(rx, ry, phi*180.0/math.Pi, largeArc, sweep, end.X, end.Y)
				} else {
					speed := func(theta float64) float64 {
						return ellipseDeriv(rx, ry, 0.0, true, theta).Length()
					}
					invL, dT := invSpeedPolynomialChebyshevApprox(10, gaussLegendre5, speed, theta1, theta2)

					startTheta := theta1
					nextLargeArc := largeArc
					for j < len(ts) && T < ts[j] && ts[j] <= T+dT {
						theta := invL(ts[j] - T)
						mid, largeArc1, largeArc2, ok := splitEllipse(rx, ry, phi, cx, cy, startTheta, theta2, theta)
						if !ok {
							panic("theta not in elliptic arc range for splitting")
						}

						q.ArcTo(rx, ry, phi*180.0/math.Pi, largeArc1, sweep, mid.X, mid.Y)
						push()
						q.MoveTo(mid.X, mid.Y)
						startTheta = theta
						nextLargeArc = largeArc2
						j++
					}
					if !equal(startTheta, theta2) {
						q.ArcTo(rx, ry, phi*180.0/math.Pi, nextLargeArc, sweep, end.X, end.Y)
					}
					T += dT
				}
			}
			i += cmdLen(cmd)
			start = end
		}
	}
	if len(q.d) > 3 {
		qs = append(qs, q)
	}
	return qs
}

//type intersection struct {
//	i int     // index into path
//	t float64 // parametric value
//}
//
//func (p *Path) SplitAtIntersections(q *Path) ([]*Path, []*Path) {
//	panic("not implemented") // TODO: implement intersections
//	selfIntersect := p == q
//	ps := []*Path{}
//	qs := []*Path{}
//	for _, pp := range p.Split() {
//		for _, qq := range q.Split() {
//			qu := []intersection{}
//			for {
//				_ = pp
//				_ = qq
//				// add to ps
//			}
//
//			if !selfIntersect {
//				sort.Slice(qu, func(i, j int) bool {
//					return qu[i].i < qu[j].i || qu[i].i == qu[j].i && qu[i].t < qu[j].t
//				})
//
//				for _, _ = range qu {
//					// add to qs
//				}
//			}
//		}
//	}
//
//	if selfIntersect {
//		return ps, ps
//	}
//	return ps, qs
//}

// Dash returns a new path that consists of dashes. Each parameter represents a length in millimeters along the original path, and will be either a dash or a space alternatingly.
func (p *Path) Dash(offset float64, d ...float64) *Path {
	if len(d) == 0 {
		return p
	}
	if len(d)%2 == 1 {
		// if d is uneven length, dash and space lengths alternate. Duplicate d so that uneven indices are always spaces
		d = append(d, d...)
	}

	i := 0 // index in d
	for d[i] < offset {
		offset -= d[i]
		i++
		if len(d) <= i {
			i = 0
		}
	}
	pos := -offset // negative if offset is halfway into dash
	if offset < 0.0 {
		dTotal := 0.0
		for _, dd := range d {
			dTotal += dd
		}
		pos = -(dTotal + offset) // handle negative offsets
	}

	q := &Path{}
	for _, ps := range p.Split() {
		length := ps.Length()
		if length <= d[i] {
			if i%2 == 0 {
				q = q.Append(ps)
			}
			continue
		}

		t := []float64{}
		for pos+d[i] < length {
			pos += d[i]
			if 0.0 < pos {
				t = append(t, pos)
			}
			i++
			if len(d) <= i {
				i = 0
			}
		}

		j0 := 0
		endsInDash := i%2 == 0
		if len(t)%2 == 1 && endsInDash || len(t)%2 == 0 && !endsInDash {
			j0 = 1
		}

		qd := &Path{}
		pd := ps.SplitAt(t...)
		for j := j0; j < len(pd)-1; j += 2 {
			qd = qd.Append(pd[j])
		}
		if endsInDash {
			if ps.Closed() {
				qd = pd[len(pd)-1].Join(qd)
			} else {
				qd = qd.Append(pd[len(pd)-1])
			}
		}
		q = q.Append(qd)
		pos -= length
	}
	return q
}

// Reverse returns a new path that is the same path as p but in the reverse direction.
func (p *Path) Reverse() *Path {
	ip := &Path{}
	if len(p.d) == 0 {
		return ip
	}

	cmds := []float64{}
	for i := 0; i < len(p.d); {
		cmds = append(cmds, p.d[i])
		i += cmdLen(p.d[i])
	}

	end := Point{p.d[len(p.d)-2], p.d[len(p.d)-1]}
	if !end.IsZero() {
		ip.MoveTo(end.X, end.Y)
	}
	start := end
	closed := false

	i := len(p.d)
	for icmd := len(cmds) - 1; icmd >= 0; icmd-- {
		cmd := cmds[icmd]
		i -= cmdLen(cmd)
		end = Point{}
		if i > 0 {
			end = Point{p.d[i-2], p.d[i-1]}
		}

		switch cmd {
		case CloseCmd:
			if !start.Equals(end) {
				ip.LineTo(end.X, end.Y)
			}
			closed = true
		case moveToCmd:
			if closed {
				ip.Close()
				closed = false
			}
			if !end.IsZero() {
				ip.MoveTo(end.X, end.Y)
			}
		case LineToCmd:
			if closed && (icmd == 0 || cmds[icmd-1] == moveToCmd) {
				ip.Close()
				closed = false
			} else {
				ip.LineTo(end.X, end.Y)
			}
		case QuadToCmd:
			cx, cy := p.d[i+1], p.d[i+2]
			ip.QuadTo(cx, cy, end.X, end.Y)
		case CubeToCmd:
			cx1, cy1 := p.d[i+3], p.d[i+4]
			cx2, cy2 := p.d[i+1], p.d[i+2]
			ip.CubeTo(cx1, cy1, cx2, cy2, end.X, end.Y)
		case ArcToCmd:
			rx, ry, phi := p.d[i+1], p.d[i+2], p.d[i+3]
			largeArc, sweep := fromArcFlags(p.d[i+4])
			ip.ArcTo(rx, ry, phi*180.0/math.Pi, largeArc, !sweep, end.X, end.Y)
		}
		start = end
	}
	if closed {
		ip.Close()
	}
	return ip
}

// Optimize returns the same path but with superfluous commands removed (such as multiple colinear LineTos). Be aware this changes the path inplace.
func (p *Path) Optimize() *Path {
	cmds := []float64{}
	for i := 0; i < len(p.d); {
		cmds = append(cmds, p.d[i])
		i += cmdLen(p.d[i])
	}

	p.i0 = 0
	i := len(p.d)
	end := Point{}
	if 0 < i {
		end = Point{p.d[i-2], p.d[i-1]}
	}
	for icmd := len(cmds) - 1; icmd >= 0; icmd-- {
		cmd := cmds[icmd]
		i -= cmdLen(cmd)
		start := Point{}
		if 0 < i {
			start = Point{p.d[i-2], p.d[i-1]}
		}
		switch cmd {
		case moveToCmd:
			if i+3 < len(p.d) && p.d[i+3] == moveToCmd || i == 0 && end.IsZero() {
				p.d = append(p.d[:i], p.d[i+3:]...)
			} else if i+3 < len(p.d) && p.d[i+3] == CloseCmd {
				p.d = append(p.d[:i], p.d[i+6:]...)
			} else if p.i0 == 0 {
				p.i0 = i
			}
		case LineToCmd:
			if start == end {
				p.d = append(p.d[:i], p.d[i+3:]...)
				cmd = NullCmd
			}
		case CloseCmd:
			if i+3 < len(p.d) && p.d[i+3] == CloseCmd {
				p.d = append(p.d[:i+3], p.d[i+6:]...) // remove last CloseCmd to ensure x,y values are valid
			}
		case QuadToCmd:
			cp := Point{p.d[i+1], p.d[i+2]}
			if cp == start || cp == end {
				p.d = append(p.d[:i+1], p.d[i+3:]...)
				p.d[i] = LineToCmd
				cmd = LineToCmd
			}
		case CubeToCmd:
			cp1 := Point{p.d[i+1], p.d[i+2]}
			cp2 := Point{p.d[i+3], p.d[i+4]}
			if (cp1 == start || cp1 == end) && (cp2 == start || cp2 == end) {
				p.d = append(p.d[:i+1], p.d[i+5:]...)
				p.d[i] = LineToCmd
				cmd = LineToCmd
			}
		case ArcToCmd:
			if start == end {
				p.d = append(p.d[:i], p.d[i+7:]...)
			}
		}

		// remove adjacent lines if they are collinear
		if cmd == LineToCmd && i+3 < len(p.d) && (p.d[i+3] == LineToCmd || p.d[i+3] == CloseCmd) {
			nextEnd := Point{p.d[i+4], p.d[i+5]}
			if p.d[i+3] == CloseCmd && end == nextEnd {
				p.d = append(p.d[:i], p.d[i+3:]...)
				p.d[i] = CloseCmd
			} else if end.Sub(start).AngleBetween(nextEnd.Sub(end)) == 0.0 {
				p.d = append(p.d[:i], p.d[i+3:]...)
			}
		}
		end = start
	}
	return p
}

////////////////////////////////////////////////////////////////

func skipCommaWhitespace(path []byte) int {
	i := 0
	for i < len(path) && (path[i] == ' ' || path[i] == ',' || path[i] == '\n' || path[i] == '\r' || path[i] == '\t') {
		i++
	}
	return i
}

func parseNum(path []byte) (float64, int) {
	i := skipCommaWhitespace(path)
	f, n := strconv.ParseFloat(path[i:])
	return f, i + n
}

// ParseSVG parses an SVG path data string.
func ParseSVG(s string) (*Path, error) {
	if len(s) == 0 {
		return &Path{}, nil
	}

	path := []byte(s)
	if path[0] < 'A' {
		return nil, fmt.Errorf("bad path: path should start with command")
	}

	cmdLens := map[byte]int{
		'M': 2,
		'Z': 0,
		'L': 2,
		'H': 1,
		'V': 1,
		'C': 6,
		'S': 4,
		'Q': 4,
		'T': 2,
		'A': 7,
	}
	f := [7]float64{}

	i := 0
	p := &Path{}
	var q, c Point
	var p0, p1 Point
	var prevCmd byte
	for i < len(path) {
		i += skipCommaWhitespace(path[i:])
		cmd := prevCmd
		if path[i] >= 'A' {
			cmd = path[i]
			i++
		}

		CMD := cmd
		if 'a' <= cmd && cmd <= 'z' {
			CMD -= 'a' - 'A'
		}
		for j := 0; j < cmdLens[CMD]; j++ {
			num, n := parseNum(path[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad path: %d numbers should follow command '%c' at position %d", cmdLens[CMD], cmd, i)
			}
			f[j] = num
			i += n
		}

		switch cmd {
		case 'M', 'm':
			p1 = Point{f[0], f[1]}
			if cmd == 'm' {
				p1 = p1.Add(p0)
			}
			p.MoveTo(p1.X, p1.Y)
		case 'Z', 'z':
			p1 = p.StartPos()
			p.Close()
		case 'L', 'l':
			p1 = Point{f[0], f[1]}
			if cmd == 'l' {
				p1 = p1.Add(p0)
			}
			p.LineTo(p1.X, p1.Y)
		case 'H', 'h':
			p1.X = f[0]
			if cmd == 'h' {
				p1.X += p0.X
			}
			p.LineTo(p1.X, p1.Y)
		case 'V', 'v':
			p1.Y = f[0]
			if cmd == 'v' {
				p1.Y += p0.Y
			}
			p.LineTo(p1.X, p1.Y)
		case 'C', 'c':
			cp1 := Point{f[0], f[1]}
			cp2 := Point{f[2], f[3]}
			p1 = Point{f[4], f[5]}
			if cmd == 'c' {
				cp1 = cp1.Add(p0)
				cp2 = cp2.Add(p0)
				p1 = p1.Add(p0)
			}
			p.CubeTo(cp1.X, cp1.Y, cp2.X, cp2.Y, p1.X, p1.Y)
			c = cp2
		case 'S', 's':
			cp1 := p0
			cp2 := Point{f[0], f[1]}
			p1 = Point{f[2], f[3]}
			if cmd == 's' {
				cp2 = cp2.Add(p0)
				p1 = p1.Add(p0)
			}
			if prevCmd == 'C' || prevCmd == 'c' || prevCmd == 'S' || prevCmd == 's' {
				cp1 = p0.Mul(2.0).Sub(c)
			}
			p.CubeTo(cp1.X, cp1.Y, cp2.X, cp2.Y, p1.X, p1.Y)
			c = cp2
		case 'Q', 'q':
			cp := Point{f[0], f[1]}
			p1 = Point{f[2], f[3]}
			if cmd == 'q' {
				cp = cp.Add(p0)
				p1 = p1.Add(p0)
			}
			p.QuadTo(cp.X, cp.Y, p1.X, p1.Y)
			q = cp
		case 'T', 't':
			cp := p0
			p1 = Point{f[0], f[1]}
			if cmd == 't' {
				p1 = p1.Add(p0)
			}
			if prevCmd == 'Q' || prevCmd == 'q' || prevCmd == 'T' || prevCmd == 't' {
				cp = p0.Mul(2.0).Sub(q)
			}
			p.QuadTo(cp.X, cp.Y, p1.X, p1.Y)
			q = cp
		case 'A', 'a':
			rx := f[0]
			ry := f[1]
			rot := f[2]
			largeArc := f[3] == 1.0
			sweep := f[4] == 1.0
			p1 = Point{f[5], f[6]}
			if cmd == 'a' {
				p1 = p1.Add(p0)
			}
			p.ArcTo(rx, ry, rot, largeArc, sweep, p1.X, p1.Y)
		default:
			return nil, fmt.Errorf("bad path: unknown command '%c' at position %d", cmd, i)
		}
		prevCmd = cmd
		p0 = p1
	}
	return p, nil
}

// String returns a string that represents the path similar to the SVG path data format (but not necessarily valid SVG).
func (p *Path) String() string {
	sb := strings.Builder{}
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		switch cmd {
		case moveToCmd:
			fmt.Fprintf(&sb, "M%.5g %.5g", p.d[i+1], p.d[i+2])
		case LineToCmd:
			fmt.Fprintf(&sb, "L%.5g %.5g", p.d[i+1], p.d[i+2])
		case QuadToCmd:
			fmt.Fprintf(&sb, "Q%.5g %.5g %.5g %.5g", p.d[i+1], p.d[i+2], p.d[i+3], p.d[i+4])
		case CubeToCmd:
			fmt.Fprintf(&sb, "C%.5g %.5g %.5g %.5g %.5g %.5g", p.d[i+1], p.d[i+2], p.d[i+3], p.d[i+4], p.d[i+5], p.d[i+6])
		case ArcToCmd:
			rot := p.d[i+3] * 180.0 / math.Pi
			largeArc, sweep := fromArcFlags(p.d[i+4])
			sLargeArc := "0"
			if largeArc {
				sLargeArc = "1"
			}
			sSweep := "0"
			if sweep {
				sSweep = "1"
			}
			fmt.Fprintf(&sb, "A%.5g %.5g %.5g %s %s %.5g %.5g", p.d[i+1], p.d[i+2], rot, sLargeArc, sSweep, p.d[i+5], p.d[i+6])
		case CloseCmd:
			fmt.Fprintf(&sb, "z")
		}
		i += cmdLen(cmd)
	}
	return sb.String()
}

// ToSVG returns a string that represents the path in the SVG path data format with minifications.
func (p *Path) ToSVG() string {
	sb := strings.Builder{}
	x, y := 0.0, 0.0
	if len(p.d) > 0 && p.d[0] != moveToCmd {
		fmt.Fprintf(&sb, "M0 0")
	}
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		switch cmd {
		case moveToCmd:
			x, y = p.d[i+1], p.d[i+2]
			fmt.Fprintf(&sb, "M%.5g %.5g", x, y)
		case LineToCmd:
			xStart, yStart := x, y
			x, y = p.d[i+1], p.d[i+2]
			if equal(x, xStart) && equal(y, yStart) {
				// nothing
			} else if equal(x, xStart) {
				fmt.Fprintf(&sb, "V%.5g", y)
			} else if equal(y, yStart) {
				fmt.Fprintf(&sb, "H%.5g", x)
			} else {
				fmt.Fprintf(&sb, "L%.5g %.5g", x, y)
			}
		case QuadToCmd:
			x, y = p.d[i+3], p.d[i+4]
			fmt.Fprintf(&sb, "Q%.5g %.5g %.5g %.5g", p.d[i+1], p.d[i+2], x, y)
		case CubeToCmd:
			x, y = p.d[i+5], p.d[i+6]
			fmt.Fprintf(&sb, "C%.5g %.5g %.5g %.5g %.5g %.5g", p.d[i+1], p.d[i+2], p.d[i+3], p.d[i+4], x, y)
		case ArcToCmd:
			rx, ry := p.d[i+1], p.d[i+2]
			rot := p.d[i+3] * 180.0 / math.Pi
			largeArc, sweep := fromArcFlags(p.d[i+4])
			x, y = p.d[i+5], p.d[i+6]
			sLargeArc := "0"
			if largeArc {
				sLargeArc = "1"
			}
			sSweep := "0"
			if sweep {
				sSweep = "1"
			}
			if 90.0 <= rot {
				rx, ry = ry, rx
				rot -= 90.0
			}
			fmt.Fprintf(&sb, "A%.5g %.5g %.5g %s %s %.5g %.5g", rx, ry, rot, sLargeArc, sSweep, p.d[i+5], p.d[i+6])
		case CloseCmd:
			x, y = p.d[i+1], p.d[i+2]
			fmt.Fprintf(&sb, "z")
		}
		i += cmdLen(cmd)
	}
	return sb.String()
}

// ToPS returns a string that represents the path in the PostScript data format.
func (p *Path) ToPS() string {
	if p.Empty() {
		return ""
	}

	sb := strings.Builder{}
	if len(p.d) > 0 && p.d[0] != moveToCmd {
		fmt.Fprintf(&sb, " 0 0 moveto")
	}

	var cmd float64
	x, y := 0.0, 0.0
	for i := 0; i < len(p.d); {
		cmd = p.d[i]
		switch cmd {
		case moveToCmd:
			x, y = p.d[i+1], p.d[i+2]
			fmt.Fprintf(&sb, " %.5f %.5f moveto", x, y)
		case LineToCmd:
			x, y = p.d[i+1], p.d[i+2]
			fmt.Fprintf(&sb, " %.5f %.5f lineto", x, y)
		case QuadToCmd, CubeToCmd:
			var start, cp1, cp2 Point
			start = Point{x, y}
			if cmd == QuadToCmd {
				x, y = p.d[i+3], p.d[i+4]
				cp1, cp2 = quadraticToCubicBezier(start, Point{p.d[i+1], p.d[i+2]}, Point{x, y})
			} else {
				cp1 = Point{p.d[i+1], p.d[i+2]}
				cp2 = Point{p.d[i+3], p.d[i+4]}
				x, y = p.d[i+5], p.d[i+6]
			}
			fmt.Fprintf(&sb, " %.5f %.5f %.5f %.5f %.5f %.5f curveto", cp1.X, cp1.Y, cp2.X, cp2.Y, x, y)
		case ArcToCmd:
			x0, y0 := x, y
			rx, ry, phi := p.d[i+1], p.d[i+2], p.d[i+3]
			largeArc, sweep := fromArcFlags(p.d[i+4])
			x, y = p.d[i+5], p.d[i+6]

			cx, cy, theta0, theta1 := ellipseToCenter(x0, y0, rx, ry, phi, largeArc, sweep, x, y)
			theta0 = theta0 * 180.0 / math.Pi
			theta1 = theta1 * 180.0 / math.Pi
			rot := phi * 180.0 / math.Pi
			isEllipse := !equal(rx, ry)

			if !equal(rot, 0.0) {
				fmt.Fprintf(&sb, " %.5f %.5f translate %.5f rotate %.5f %.5f translate", cx, cy, rot, -cx, -cy)
			}
			if isEllipse {
				fmt.Fprintf(&sb, " %.5f %.5f %.5f %.5f %.5f %.5f ellipse", cx, cy, rx, ry, theta0, theta1)
			} else {
				fmt.Fprintf(&sb, " %.5f %.5f %.5f %.5f %.5f arc", cx, cy, rx, theta0, theta1)
			}
			if !sweep {
				fmt.Fprintf(&sb, "n")
			}
			if !equal(rot, 0.0) {
				fmt.Fprintf(&sb, " initmatrix")
			}
		case CloseCmd:
			x, y = p.d[i+1], p.d[i+2]
			fmt.Fprintf(&sb, " closepath")
		}
		i += cmdLen(cmd)
	}
	if cmd != CloseCmd {
		fmt.Fprintf(&sb, " closepath")
	}
	return sb.String()[1:] // remove the first space
}

// ToPDF returns a string that represents the path in the PDF data format.
func (p *Path) ToPDF() string {
	if p.Empty() {
		return ""
	}
	p = p.Copy().Replace(nil, nil, ellipseToBeziers)

	sb := strings.Builder{}
	if len(p.d) > 0 && p.d[0] != moveToCmd {
		fmt.Fprintf(&sb, " 0 0 m")
	}

	x, y := 0.0, 0.0
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		switch cmd {
		case moveToCmd:
			x, y = p.d[i+1], p.d[i+2]
			fmt.Fprintf(&sb, " %.5f %.5f m", x, y)
		case LineToCmd:
			x, y = p.d[i+1], p.d[i+2]
			fmt.Fprintf(&sb, " %.5f %.5f l", x, y)
		case QuadToCmd, CubeToCmd:
			var start, cp1, cp2 Point
			start = Point{x, y}
			if cmd == QuadToCmd {
				x, y = p.d[i+3], p.d[i+4]
				cp1, cp2 = quadraticToCubicBezier(start, Point{p.d[i+1], p.d[i+2]}, Point{x, y})
			} else {
				cp1 = Point{p.d[i+1], p.d[i+2]}
				cp2 = Point{p.d[i+3], p.d[i+4]}
				x, y = p.d[i+5], p.d[i+6]
			}
			fmt.Fprintf(&sb, " %.5f %.5f %.5f %.5f %.5f %.5f c", cp1.X, cp1.Y, cp2.X, cp2.Y, x, y)
		case ArcToCmd:
			panic("arcs should have been replaced")
		case CloseCmd:
			x, y = p.d[i+1], p.d[i+2]
			fmt.Fprintf(&sb, " h")
		}
		i += cmdLen(cmd)
	}
	return sb.String()[1:] // remove the first space
}

// ToRasterizer rasterizes the path using the given rasterizer with dpm the dots-per-millimeter.
func (p *Path) ToRasterizer(ras *vector.Rasterizer, dpm float64) {
	p = p.Copy().Replace(nil, nil, ellipseToBeziers)

	closed := false
	dy := float64(ras.Bounds().Size().Y)
	for i := 0; i < len(p.d); {
		cmd := p.d[i]
		closed = false
		switch cmd {
		case moveToCmd:
			ras.MoveTo(float32(p.d[i+1]*dpm), float32(dy-p.d[i+2]*dpm))
		case LineToCmd:
			ras.LineTo(float32(p.d[i+1]*dpm), float32(dy-p.d[i+2]*dpm))
		case QuadToCmd:
			ras.QuadTo(float32(p.d[i+1]*dpm), float32(dy-p.d[i+2]*dpm), float32(p.d[i+3]*dpm), float32(dy-p.d[i+4]*dpm))
		case CubeToCmd:
			ras.CubeTo(float32(p.d[i+1]*dpm), float32(dy-p.d[i+2]*dpm), float32(p.d[i+3]*dpm), float32(dy-p.d[i+4]*dpm), float32(p.d[i+5]*dpm), float32(dy-p.d[i+6]*dpm))
		case ArcToCmd:
			panic("arcs should have been replaced")
		case CloseCmd:
			ras.ClosePath()
			closed = true
		}
		i += cmdLen(cmd)
	}
	if !closed {
		// implicitly close path
		ras.ClosePath()
	}
}
