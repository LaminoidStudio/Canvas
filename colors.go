package canvas

import (
	"image/color"
	"math"
)

// ColorSpace defines the color space within the RGB color model. All colors passed to this library are assumed to be in the sRGB color space, which is a ubiquitous assumption in most software. This works great for most applications, but fails when blending semi-transparent layers. See an elaborate explaination at https://blog.johnnovak.net/2016/09/21/what-every-coder-should-know-about-gamma/, which goes into depth of the problems of using sRGB for blending and the need for gamma correction. In short, we need to transform the colors, which are in the sRGB color space, to the linear color space, perform blending, and then transform them back to the sRGB color space.
// Unfortunately, almost all software does blending the wrong way (all PDF renderers and browsers I've tested), so by default this library will do the same by using LinearColorSpace which does no conversion from sRGB to linear and back but blends directly in sRGB. Or in other words, it assumes that colors are given in the linear color space and that the output image is expected to be in the linear color space as well. For technical correctness we should really be using the SRGBColorSpace, which will convert from sRGB to linear space, do blending in linear space, and then go back to sRGB space.
type ColorSpace interface {
	ToLinear(color.Color) color.RGBA
	FromLinear(color.Color) color.RGBA
}

// DefaultColorSpace is set to LinearColorSpace to match other renderers.
var DefaultColorSpace ColorSpace = LinearColorSpace{}

// LinearColorSpace is the default color space that does not do color space conversion for blending purposes. This is only correct if the input colors and output images are assumed to be in the linear color space so that blending is in linear space as well. In general though, we assume that input colors and output images are using the sRGB color space almost ubiquitously, resulting in blending in sRGB space which is wrong! Even though it is technically incorrect, many PDF viewers and browsers do this anyway.
type LinearColorSpace struct{}

func (LinearColorSpace) ToLinear(col color.Color) color.RGBA {
	if rgba, ok := col.(color.RGBA); ok {
		return rgba
	}
	R, G, B, A := col.RGBA()
	return color.RGBA{uint8(R >> 8), uint8(G >> 8), uint8(B >> 8), uint8(A >> 8)}
}

func (LinearColorSpace) FromLinear(col color.Color) color.RGBA {
	if rgba, ok := col.(color.RGBA); ok {
		return rgba
	}
	R, G, B, A := col.RGBA()
	return color.RGBA{uint8(R >> 8), uint8(G >> 8), uint8(B >> 8), uint8(A >> 8)}
}

// GammaColorSpace assumes that input colors and output images are gamma-corrected with the given gamma value. The sRGB space uses a gamma=2.4 for most of the curve, but will on average have a gamma=2.2 best approximating the sRGB curve. See https://en.wikipedia.org/wiki/SRGB#The_sRGB_transfer_function_(%22gamma%22). According to https://www.puredevsoftware.com/blog/2019/01/22/sub-pixel-gamma-correct-font-rendering/, a gamma=1.43 is recommended for fonts.
type GammaColorSpace struct {
	Gamma float64
}

func (cs GammaColorSpace) ToLinear(col color.Color) color.RGBA {
	R, G, B, A := col.RGBA()
	r := math.Pow(float64(R)/float64(A), cs.Gamma)
	g := math.Pow(float64(G)/float64(A), cs.Gamma)
	b := math.Pow(float64(B)/float64(A), cs.Gamma)
	a := float64(A) / 0xffff
	return color.RGBA{
		uint8(r*a*255.0 + 0.5),
		uint8(g*a*255.0 + 0.5),
		uint8(b*a*255.0 + 0.5),
		uint8(a*255.0 + 0.5),
	}
}

func (cs GammaColorSpace) FromLinear(col color.Color) color.RGBA {
	R, G, B, A := col.RGBA()
	r := math.Pow(float64(R)/float64(A), 1.0/cs.Gamma)
	g := math.Pow(float64(G)/float64(A), 1.0/cs.Gamma)
	b := math.Pow(float64(B)/float64(A), 1.0/cs.Gamma)
	a := float64(A) / 0xffff
	return color.RGBA{
		uint8(r*a*255.0 + 0.5),
		uint8(g*a*255.0 + 0.5),
		uint8(b*a*255.0 + 0.5),
		uint8(a*255.0 + 0.5),
	}
}

// SRGBColorSpace assumes that input colors and output images are in the sRGB color space (ubiquitous in almost all applications), which implies that for blending we need to convert to the linear color space, do blending, and then convert back to the sRGB color space. This will give technically correct blending, but may differ from common PDF viewer and browsers (which are wrong).
type SRGBColorSpace struct{}

func (SRGBColorSpace) ToLinear(col color.Color) color.RGBA {
	sRGBToLinear := func(c float64) float64 {
		// Formula from EXT_sRGB.
		if c <= 0.04045 {
			return c / 12.92
		} else {
			return math.Pow((c+0.055)/1.055, 2.4)
		}
	}

	R, G, B, A := col.RGBA()
	r := sRGBToLinear(float64(R) / float64(A))
	g := sRGBToLinear(float64(G) / float64(A))
	b := sRGBToLinear(float64(B) / float64(A))
	a := float64(A) / 0xffff
	return color.RGBA{
		uint8(r*a*255.0 + 0.5),
		uint8(g*a*255.0 + 0.5),
		uint8(b*a*255.0 + 0.5),
		uint8(a*255.0 + 0.5),
	}
}

func (SRGBColorSpace) FromLinear(col color.Color) color.RGBA {
	linearTosRGB := func(c float64) float64 {
		// Formula from EXT_sRGB.
		switch {
		case c <= 0.0:
			return 0.0
		case 0 < c && c < 0.0031308:
			return 12.92 * c
		case 0.0031308 <= c && c < 1:
			return 1.055*math.Pow(c, 0.41666) - 0.055
		}
		return 1.0
	}

	R, G, B, A := col.RGBA()
	r := linearTosRGB(float64(R) / float64(A))
	g := linearTosRGB(float64(G) / float64(A))
	b := linearTosRGB(float64(B) / float64(A))
	a := float64(A) / 0xffff
	return color.RGBA{
		uint8(r*a*255.0 + 0.5),
		uint8(g*a*255.0 + 0.5),
		uint8(b*a*255.0 + 0.5),
		uint8(a*255.0 + 0.5),
	}
}

func Hex(s string) color.RGBA {
	if 0 < len(s) && s[0] == '#' {
		s = s[1:]
	}
	h := make([]uint8, len(s))
	for i, c := range s {
		if '0' <= c && c <= '9' {
			h[i] = uint8(c - '0')
		} else if 'a' <= c && c <= 'f' {
			h[i] = 10 + uint8(c-'a')
		} else if 'A' <= c && c <= 'F' {
			h[i] = 10 + uint8(c-'A')
		}
	}
	if len(s) == 3 {
		return color.RGBA{h[0]*16 + h[0], h[1]*16 + h[1], h[2]*16 + h[2], 0xff}
	} else if len(s) == 4 {
		a := float64(h[3]*16+h[0]) / 255.0
		return color.RGBA{
			uint8(a * float64(h[0]*16+h[0])),
			uint8(a * float64(h[1]*16+h[1])),
			uint8(a * float64(h[2]*16+h[2])),
			h[3]*16 + h[3],
		}
	} else if len(s) == 6 {
		return color.RGBA{h[0]*16 + h[1], h[2]*16 + h[3], h[4]*16 + h[5], 0xff}
	} else if len(s) == 8 {
		a := float64(h[6]*16+h[7]) / 255.0
		return color.RGBA{
			uint8(a * float64(h[0]*16+h[1])),
			uint8(a * float64(h[2]*16+h[3])),
			uint8(a * float64(h[4]*16+h[5])),
			h[6]*16 + h[7],
		}
	}
	return Black
}

func RGBA(r, g, b uint8, a float64) color.RGBA {
	return color.RGBA{
		uint8(a * float64(r)),
		uint8(a * float64(g)),
		uint8(a * float64(b)),
		uint8(a * 255.0),
	}
}

// Transparent when used as a fill or stroke color will indicate that the fill or stroke will not be drawn.
var Transparent = color.RGBA{0x00, 0x00, 0x00, 0x00} // rgba(0, 0, 0, 0)

// from https://golang.org/x/image/colornames
var (
	Aliceblue            = color.RGBA{0xf0, 0xf8, 0xff, 0xff} // rgb(240, 248, 255)
	Antiquewhite         = color.RGBA{0xfa, 0xeb, 0xd7, 0xff} // rgb(250, 235, 215)
	Aqua                 = color.RGBA{0x00, 0xff, 0xff, 0xff} // rgb(0, 255, 255)
	Aquamarine           = color.RGBA{0x7f, 0xff, 0xd4, 0xff} // rgb(127, 255, 212)
	Azure                = color.RGBA{0xf0, 0xff, 0xff, 0xff} // rgb(240, 255, 255)
	Beige                = color.RGBA{0xf5, 0xf5, 0xdc, 0xff} // rgb(245, 245, 220)
	Bisque               = color.RGBA{0xff, 0xe4, 0xc4, 0xff} // rgb(255, 228, 196)
	Black                = color.RGBA{0x00, 0x00, 0x00, 0xff} // rgb(0, 0, 0)
	Blanchedalmond       = color.RGBA{0xff, 0xeb, 0xcd, 0xff} // rgb(255, 235, 205)
	Blue                 = color.RGBA{0x00, 0x00, 0xff, 0xff} // rgb(0, 0, 255)
	Blueviolet           = color.RGBA{0x8a, 0x2b, 0xe2, 0xff} // rgb(138, 43, 226)
	Brown                = color.RGBA{0xa5, 0x2a, 0x2a, 0xff} // rgb(165, 42, 42)
	Burlywood            = color.RGBA{0xde, 0xb8, 0x87, 0xff} // rgb(222, 184, 135)
	Cadetblue            = color.RGBA{0x5f, 0x9e, 0xa0, 0xff} // rgb(95, 158, 160)
	Chartreuse           = color.RGBA{0x7f, 0xff, 0x00, 0xff} // rgb(127, 255, 0)
	Chocolate            = color.RGBA{0xd2, 0x69, 0x1e, 0xff} // rgb(210, 105, 30)
	Coral                = color.RGBA{0xff, 0x7f, 0x50, 0xff} // rgb(255, 127, 80)
	Cornflowerblue       = color.RGBA{0x64, 0x95, 0xed, 0xff} // rgb(100, 149, 237)
	Cornsilk             = color.RGBA{0xff, 0xf8, 0xdc, 0xff} // rgb(255, 248, 220)
	Crimson              = color.RGBA{0xdc, 0x14, 0x3c, 0xff} // rgb(220, 20, 60)
	Cyan                 = color.RGBA{0x00, 0xff, 0xff, 0xff} // rgb(0, 255, 255)
	Darkblue             = color.RGBA{0x00, 0x00, 0x8b, 0xff} // rgb(0, 0, 139)
	Darkcyan             = color.RGBA{0x00, 0x8b, 0x8b, 0xff} // rgb(0, 139, 139)
	Darkgoldenrod        = color.RGBA{0xb8, 0x86, 0x0b, 0xff} // rgb(184, 134, 11)
	Darkgray             = color.RGBA{0xa9, 0xa9, 0xa9, 0xff} // rgb(169, 169, 169)
	Darkgreen            = color.RGBA{0x00, 0x64, 0x00, 0xff} // rgb(0, 100, 0)
	Darkgrey             = color.RGBA{0xa9, 0xa9, 0xa9, 0xff} // rgb(169, 169, 169)
	Darkkhaki            = color.RGBA{0xbd, 0xb7, 0x6b, 0xff} // rgb(189, 183, 107)
	Darkmagenta          = color.RGBA{0x8b, 0x00, 0x8b, 0xff} // rgb(139, 0, 139)
	Darkolivegreen       = color.RGBA{0x55, 0x6b, 0x2f, 0xff} // rgb(85, 107, 47)
	Darkorange           = color.RGBA{0xff, 0x8c, 0x00, 0xff} // rgb(255, 140, 0)
	Darkorchid           = color.RGBA{0x99, 0x32, 0xcc, 0xff} // rgb(153, 50, 204)
	Darkred              = color.RGBA{0x8b, 0x00, 0x00, 0xff} // rgb(139, 0, 0)
	Darksalmon           = color.RGBA{0xe9, 0x96, 0x7a, 0xff} // rgb(233, 150, 122)
	Darkseagreen         = color.RGBA{0x8f, 0xbc, 0x8f, 0xff} // rgb(143, 188, 143)
	Darkslateblue        = color.RGBA{0x48, 0x3d, 0x8b, 0xff} // rgb(72, 61, 139)
	Darkslategray        = color.RGBA{0x2f, 0x4f, 0x4f, 0xff} // rgb(47, 79, 79)
	Darkslategrey        = color.RGBA{0x2f, 0x4f, 0x4f, 0xff} // rgb(47, 79, 79)
	Darkturquoise        = color.RGBA{0x00, 0xce, 0xd1, 0xff} // rgb(0, 206, 209)
	Darkviolet           = color.RGBA{0x94, 0x00, 0xd3, 0xff} // rgb(148, 0, 211)
	Deeppink             = color.RGBA{0xff, 0x14, 0x93, 0xff} // rgb(255, 20, 147)
	Deepskyblue          = color.RGBA{0x00, 0xbf, 0xff, 0xff} // rgb(0, 191, 255)
	Dimgray              = color.RGBA{0x69, 0x69, 0x69, 0xff} // rgb(105, 105, 105)
	Dimgrey              = color.RGBA{0x69, 0x69, 0x69, 0xff} // rgb(105, 105, 105)
	Dodgerblue           = color.RGBA{0x1e, 0x90, 0xff, 0xff} // rgb(30, 144, 255)
	Firebrick            = color.RGBA{0xb2, 0x22, 0x22, 0xff} // rgb(178, 34, 34)
	Floralwhite          = color.RGBA{0xff, 0xfa, 0xf0, 0xff} // rgb(255, 250, 240)
	Forestgreen          = color.RGBA{0x22, 0x8b, 0x22, 0xff} // rgb(34, 139, 34)
	Fuchsia              = color.RGBA{0xff, 0x00, 0xff, 0xff} // rgb(255, 0, 255)
	Gainsboro            = color.RGBA{0xdc, 0xdc, 0xdc, 0xff} // rgb(220, 220, 220)
	Ghostwhite           = color.RGBA{0xf8, 0xf8, 0xff, 0xff} // rgb(248, 248, 255)
	Gold                 = color.RGBA{0xff, 0xd7, 0x00, 0xff} // rgb(255, 215, 0)
	Goldenrod            = color.RGBA{0xda, 0xa5, 0x20, 0xff} // rgb(218, 165, 32)
	Gray                 = color.RGBA{0x80, 0x80, 0x80, 0xff} // rgb(128, 128, 128)
	Green                = color.RGBA{0x00, 0x80, 0x00, 0xff} // rgb(0, 128, 0)
	Greenyellow          = color.RGBA{0xad, 0xff, 0x2f, 0xff} // rgb(173, 255, 47)
	Grey                 = color.RGBA{0x80, 0x80, 0x80, 0xff} // rgb(128, 128, 128)
	Honeydew             = color.RGBA{0xf0, 0xff, 0xf0, 0xff} // rgb(240, 255, 240)
	Hotpink              = color.RGBA{0xff, 0x69, 0xb4, 0xff} // rgb(255, 105, 180)
	Indianred            = color.RGBA{0xcd, 0x5c, 0x5c, 0xff} // rgb(205, 92, 92)
	Indigo               = color.RGBA{0x4b, 0x00, 0x82, 0xff} // rgb(75, 0, 130)
	Ivory                = color.RGBA{0xff, 0xff, 0xf0, 0xff} // rgb(255, 255, 240)
	Khaki                = color.RGBA{0xf0, 0xe6, 0x8c, 0xff} // rgb(240, 230, 140)
	Lavender             = color.RGBA{0xe6, 0xe6, 0xfa, 0xff} // rgb(230, 230, 250)
	Lavenderblush        = color.RGBA{0xff, 0xf0, 0xf5, 0xff} // rgb(255, 240, 245)
	Lawngreen            = color.RGBA{0x7c, 0xfc, 0x00, 0xff} // rgb(124, 252, 0)
	Lemonchiffon         = color.RGBA{0xff, 0xfa, 0xcd, 0xff} // rgb(255, 250, 205)
	Lightblue            = color.RGBA{0xad, 0xd8, 0xe6, 0xff} // rgb(173, 216, 230)
	Lightcoral           = color.RGBA{0xf0, 0x80, 0x80, 0xff} // rgb(240, 128, 128)
	Lightcyan            = color.RGBA{0xe0, 0xff, 0xff, 0xff} // rgb(224, 255, 255)
	Lightgoldenrodyellow = color.RGBA{0xfa, 0xfa, 0xd2, 0xff} // rgb(250, 250, 210)
	Lightgray            = color.RGBA{0xd3, 0xd3, 0xd3, 0xff} // rgb(211, 211, 211)
	Lightgreen           = color.RGBA{0x90, 0xee, 0x90, 0xff} // rgb(144, 238, 144)
	Lightgrey            = color.RGBA{0xd3, 0xd3, 0xd3, 0xff} // rgb(211, 211, 211)
	Lightpink            = color.RGBA{0xff, 0xb6, 0xc1, 0xff} // rgb(255, 182, 193)
	Lightsalmon          = color.RGBA{0xff, 0xa0, 0x7a, 0xff} // rgb(255, 160, 122)
	Lightseagreen        = color.RGBA{0x20, 0xb2, 0xaa, 0xff} // rgb(32, 178, 170)
	Lightskyblue         = color.RGBA{0x87, 0xce, 0xfa, 0xff} // rgb(135, 206, 250)
	Lightslategray       = color.RGBA{0x77, 0x88, 0x99, 0xff} // rgb(119, 136, 153)
	Lightslategrey       = color.RGBA{0x77, 0x88, 0x99, 0xff} // rgb(119, 136, 153)
	Lightsteelblue       = color.RGBA{0xb0, 0xc4, 0xde, 0xff} // rgb(176, 196, 222)
	Lightyellow          = color.RGBA{0xff, 0xff, 0xe0, 0xff} // rgb(255, 255, 224)
	Lime                 = color.RGBA{0x00, 0xff, 0x00, 0xff} // rgb(0, 255, 0)
	Limegreen            = color.RGBA{0x32, 0xcd, 0x32, 0xff} // rgb(50, 205, 50)
	Linen                = color.RGBA{0xfa, 0xf0, 0xe6, 0xff} // rgb(250, 240, 230)
	Magenta              = color.RGBA{0xff, 0x00, 0xff, 0xff} // rgb(255, 0, 255)
	Maroon               = color.RGBA{0x80, 0x00, 0x00, 0xff} // rgb(128, 0, 0)
	Mediumaquamarine     = color.RGBA{0x66, 0xcd, 0xaa, 0xff} // rgb(102, 205, 170)
	Mediumblue           = color.RGBA{0x00, 0x00, 0xcd, 0xff} // rgb(0, 0, 205)
	Mediumorchid         = color.RGBA{0xba, 0x55, 0xd3, 0xff} // rgb(186, 85, 211)
	Mediumpurple         = color.RGBA{0x93, 0x70, 0xdb, 0xff} // rgb(147, 112, 219)
	Mediumseagreen       = color.RGBA{0x3c, 0xb3, 0x71, 0xff} // rgb(60, 179, 113)
	Mediumslateblue      = color.RGBA{0x7b, 0x68, 0xee, 0xff} // rgb(123, 104, 238)
	Mediumspringgreen    = color.RGBA{0x00, 0xfa, 0x9a, 0xff} // rgb(0, 250, 154)
	Mediumturquoise      = color.RGBA{0x48, 0xd1, 0xcc, 0xff} // rgb(72, 209, 204)
	Mediumvioletred      = color.RGBA{0xc7, 0x15, 0x85, 0xff} // rgb(199, 21, 133)
	Midnightblue         = color.RGBA{0x19, 0x19, 0x70, 0xff} // rgb(25, 25, 112)
	Mintcream            = color.RGBA{0xf5, 0xff, 0xfa, 0xff} // rgb(245, 255, 250)
	Mistyrose            = color.RGBA{0xff, 0xe4, 0xe1, 0xff} // rgb(255, 228, 225)
	Moccasin             = color.RGBA{0xff, 0xe4, 0xb5, 0xff} // rgb(255, 228, 181)
	Navajowhite          = color.RGBA{0xff, 0xde, 0xad, 0xff} // rgb(255, 222, 173)
	Navy                 = color.RGBA{0x00, 0x00, 0x80, 0xff} // rgb(0, 0, 128)
	Oldlace              = color.RGBA{0xfd, 0xf5, 0xe6, 0xff} // rgb(253, 245, 230)
	Olive                = color.RGBA{0x80, 0x80, 0x00, 0xff} // rgb(128, 128, 0)
	Olivedrab            = color.RGBA{0x6b, 0x8e, 0x23, 0xff} // rgb(107, 142, 35)
	Orange               = color.RGBA{0xff, 0xa5, 0x00, 0xff} // rgb(255, 165, 0)
	Orangered            = color.RGBA{0xff, 0x45, 0x00, 0xff} // rgb(255, 69, 0)
	Orchid               = color.RGBA{0xda, 0x70, 0xd6, 0xff} // rgb(218, 112, 214)
	Palegoldenrod        = color.RGBA{0xee, 0xe8, 0xaa, 0xff} // rgb(238, 232, 170)
	Palegreen            = color.RGBA{0x98, 0xfb, 0x98, 0xff} // rgb(152, 251, 152)
	Paleturquoise        = color.RGBA{0xaf, 0xee, 0xee, 0xff} // rgb(175, 238, 238)
	Palevioletred        = color.RGBA{0xdb, 0x70, 0x93, 0xff} // rgb(219, 112, 147)
	Papayawhip           = color.RGBA{0xff, 0xef, 0xd5, 0xff} // rgb(255, 239, 213)
	Peachpuff            = color.RGBA{0xff, 0xda, 0xb9, 0xff} // rgb(255, 218, 185)
	Peru                 = color.RGBA{0xcd, 0x85, 0x3f, 0xff} // rgb(205, 133, 63)
	Pink                 = color.RGBA{0xff, 0xc0, 0xcb, 0xff} // rgb(255, 192, 203)
	Plum                 = color.RGBA{0xdd, 0xa0, 0xdd, 0xff} // rgb(221, 160, 221)
	Powderblue           = color.RGBA{0xb0, 0xe0, 0xe6, 0xff} // rgb(176, 224, 230)
	Purple               = color.RGBA{0x80, 0x00, 0x80, 0xff} // rgb(128, 0, 128)
	Red                  = color.RGBA{0xff, 0x00, 0x00, 0xff} // rgb(255, 0, 0)
	Rosybrown            = color.RGBA{0xbc, 0x8f, 0x8f, 0xff} // rgb(188, 143, 143)
	Royalblue            = color.RGBA{0x41, 0x69, 0xe1, 0xff} // rgb(65, 105, 225)
	Saddlebrown          = color.RGBA{0x8b, 0x45, 0x13, 0xff} // rgb(139, 69, 19)
	Salmon               = color.RGBA{0xfa, 0x80, 0x72, 0xff} // rgb(250, 128, 114)
	Sandybrown           = color.RGBA{0xf4, 0xa4, 0x60, 0xff} // rgb(244, 164, 96)
	Seagreen             = color.RGBA{0x2e, 0x8b, 0x57, 0xff} // rgb(46, 139, 87)
	Seashell             = color.RGBA{0xff, 0xf5, 0xee, 0xff} // rgb(255, 245, 238)
	Sienna               = color.RGBA{0xa0, 0x52, 0x2d, 0xff} // rgb(160, 82, 45)
	Silver               = color.RGBA{0xc0, 0xc0, 0xc0, 0xff} // rgb(192, 192, 192)
	Skyblue              = color.RGBA{0x87, 0xce, 0xeb, 0xff} // rgb(135, 206, 235)
	Slateblue            = color.RGBA{0x6a, 0x5a, 0xcd, 0xff} // rgb(106, 90, 205)
	Slategray            = color.RGBA{0x70, 0x80, 0x90, 0xff} // rgb(112, 128, 144)
	Slategrey            = color.RGBA{0x70, 0x80, 0x90, 0xff} // rgb(112, 128, 144)
	Snow                 = color.RGBA{0xff, 0xfa, 0xfa, 0xff} // rgb(255, 250, 250)
	Springgreen          = color.RGBA{0x00, 0xff, 0x7f, 0xff} // rgb(0, 255, 127)
	Steelblue            = color.RGBA{0x46, 0x82, 0xb4, 0xff} // rgb(70, 130, 180)
	Tan                  = color.RGBA{0xd2, 0xb4, 0x8c, 0xff} // rgb(210, 180, 140)
	Teal                 = color.RGBA{0x00, 0x80, 0x80, 0xff} // rgb(0, 128, 128)
	Thistle              = color.RGBA{0xd8, 0xbf, 0xd8, 0xff} // rgb(216, 191, 216)
	Tomato               = color.RGBA{0xff, 0x63, 0x47, 0xff} // rgb(255, 99, 71)
	Turquoise            = color.RGBA{0x40, 0xe0, 0xd0, 0xff} // rgb(64, 224, 208)
	Violet               = color.RGBA{0xee, 0x82, 0xee, 0xff} // rgb(238, 130, 238)
	Wheat                = color.RGBA{0xf5, 0xde, 0xb3, 0xff} // rgb(245, 222, 179)
	White                = color.RGBA{0xff, 0xff, 0xff, 0xff} // rgb(255, 255, 255)
	Whitesmoke           = color.RGBA{0xf5, 0xf5, 0xf5, 0xff} // rgb(245, 245, 245)
	Yellow               = color.RGBA{0xff, 0xff, 0x00, 0xff} // rgb(255, 255, 0)
	Yellowgreen          = color.RGBA{0x9a, 0xcd, 0x32, 0xff} // rgb(154, 205, 50)
)
