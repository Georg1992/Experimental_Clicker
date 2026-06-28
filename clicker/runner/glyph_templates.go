package runner

// GlyphTemplate represents a single digit or character template.
type GlyphTemplate struct {
	Char   rune    // '0'-'9' or '/'
	Width  int     // Template width
	Height int     // Template height
	Pixels [][]bool // Binary pixel data
}

// TemplateLibrary contains all digit and separator templates.
type TemplateLibrary struct {
	templates map[rune]*GlyphTemplate
}

// NewTemplateLibrary creates a new template library with all digit and separator templates.
func NewTemplateLibrary() *TemplateLibrary {
	lib := &TemplateLibrary{
		templates: make(map[rune]*GlyphTemplate),
	}
	
	// Register all templates
	for i := '0'; i <= '9'; i++ {
		lib.templates[i] = getDigitTemplate(i)
	}
	lib.templates['/'] = getSeparatorTemplate()
	
	return lib
}

// GetTemplate retrieves a template for a character.
func (lib *TemplateLibrary) GetTemplate(ch rune) *GlyphTemplate {
	return lib.templates[ch]
}

// AllTemplates returns all registered templates.
func (lib *TemplateLibrary) AllTemplates() map[rune]*GlyphTemplate {
	return lib.templates
}

// getDigitTemplate returns the bitmap template for a digit.
// These are simple 7-segment-like patterns that work for Ragnarok's fixed font.
func getDigitTemplate(digit rune) *GlyphTemplate {
	// For Ragnarok's typical small fixed font, digits are approximately 8x14 pixels.
	// These templates are simplified representations - in production they would be
	// extracted from actual game screenshots and optimized for the exact font.
	
	switch digit {
	case '0':
		return &GlyphTemplate{
			Char:   '0',
			Width:  8,
			Height: 14,
			Pixels: boolArray(
				"  ##### ",
				" #     #",
				"#       #",
				"#       #",
				"#       #",
				"#       #",
				"#       #",
				"#       #",
				"#       #",
				"#       #",
				"#       #",
				" #     #",
				" #     #",
				"  ##### ",
			),
		}
	case '1':
		return &GlyphTemplate{
			Char:   '1',
			Width:  6,
			Height: 14,
			Pixels: boolArray(
				"    # ",
				"   ## ",
				"  ### ",
				"    # ",
				"    # ",
				"    # ",
				"    # ",
				"    # ",
				"    # ",
				"    # ",
				"    # ",
				"    # ",
				"    # ",
				" ##### ",
			),
		}
	case '2':
		return &GlyphTemplate{
			Char:   '2',
			Width:  8,
			Height: 14,
			Pixels: boolArray(
				"  ##### ",
				" #     #",
				"#       #",
				"        #",
				"        #",
				"       # ",
				"      #  ",
				"     #   ",
				"    #    ",
				"   #     ",
				"  #      ",
				" #       ",
				"#        ",
				"#########",
			),
		}
	case '3':
		return &GlyphTemplate{
			Char:   '3',
			Width:  8,
			Height: 14,
			Pixels: boolArray(
				"  ##### ",
				" #     #",
				"#       #",
				"        #",
				"        #",
				"   #### ",
				"        #",
				"        #",
				"        #",
				"        #",
				"#       #",
				" #     #",
				" #     #",
				"  ##### ",
			),
		}
	case '4':
		return &GlyphTemplate{
			Char:   '4',
			Width:  8,
			Height: 14,
			Pixels: boolArray(
				"       # ",
				"      ## ",
				"     ### ",
				"    # ## ",
				"   #  ## ",
				"  #   ## ",
				" #    ## ",
				"#     ## ",
				"##########",
				"      ## ",
				"      ## ",
				"      ## ",
				"      ## ",
				"      ## ",
			),
		}
	case '5':
		return &GlyphTemplate{
			Char:   '5',
			Width:  8,
			Height: 14,
			Pixels: boolArray(
				"#########",
				"#        ",
				"#        ",
				"#        ",
				"#        ",
				" ###### ",
				"        #",
				"        #",
				"        #",
				"        #",
				"#       #",
				" #     #",
				" #     #",
				"  ##### ",
			),
		}
	case '6':
		return &GlyphTemplate{
			Char:   '6',
			Width:  8,
			Height: 14,
			Pixels: boolArray(
				"   ##### ",
				"  #     #",
				" #       ",
				"#        ",
				"#        ",
				"# ##### ",
				"##      #",
				"#       #",
				"#       #",
				"#       #",
				"#       #",
				" #     #",
				" #     #",
				"  ##### ",
			),
		}
	case '7':
		return &GlyphTemplate{
			Char:   '7',
			Width:  8,
			Height: 14,
			Pixels: boolArray(
				"#########",
				"        #",
				"        #",
				"       # ",
				"      #  ",
				"      #  ",
				"     #   ",
				"    #    ",
				"    #    ",
				"   #     ",
				"  #      ",
				"  #      ",
				" #       ",
				" #       ",
			),
		}
	case '8':
		return &GlyphTemplate{
			Char:   '8',
			Width:  8,
			Height: 14,
			Pixels: boolArray(
				"  ##### ",
				" #     #",
				"#       #",
				"#       #",
				" #     # ",
				"  ##### ",
				" #     # ",
				"#       #",
				"#       #",
				"#       #",
				"#       #",
				" #     #",
				" #     #",
				"  ##### ",
			),
		}
	case '9':
		return &GlyphTemplate{
			Char:   '9',
			Width:  8,
			Height: 14,
			Pixels: boolArray(
				"  ##### ",
				" #     #",
				"#       #",
				"#       #",
				"#       #",
				"#       #",
				" #     ##",
				"  ##### #",
				"        #",
				"        #",
				"       # ",
				" #     #",
				" #     #",
				"  ##### ",
			),
		}
	}
	
	return nil // Unknown digit
}

// getSeparatorTemplate returns the template for the '/' separator.
func getSeparatorTemplate() *GlyphTemplate {
	return &GlyphTemplate{
		Char:   '/',
		Width:  6,
		Height: 14,
		Pixels: boolArray(
			"        #",
			"        #",
			"       # ",
			"       # ",
			"      #  ",
			"      #  ",
			"     #   ",
			"     #   ",
			"    #    ",
			"    #    ",
			"   #     ",
			"   #     ",
			"  #      ",
			"  #      ",
		),
	}
}

// boolArray converts a string array (# = true, space = false) to bool array.
func boolArray(lines ...string) [][]bool {
	result := make([][]bool, len(lines))
	for i, line := range lines {
		result[i] = make([]bool, len(line))
		for j, ch := range line {
			result[i][j] = ch == '#'
		}
	}
	return result
}
