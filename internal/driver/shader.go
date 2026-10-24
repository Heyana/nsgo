package driver

// ShaderProgram is a fragment shader an app compiled: its source in Go's
// syntax (Kage), which each platform translates for its GPU once and
// keeps while the program is in use. Once handed to the platform it never
// changes, so platforms key what they made from it by its pointer.
type ShaderProgram struct {
	Source string
}

// Shader is what a shader view draws (ItemShader): Program with the
// values of its uniforms.
type Shader struct {
	Program *ShaderProgram
	// Lanes are the program's own uniforms, after those the platform
	// sets, lane by lane in the order they are declared: a vector's
	// components, a matrix's columns and an array's elements in turn.
	// An int, uint or bool lane takes the value's integer part.
	Lanes []float64
	// Nodes are the lanes animated nodes drive over Lanes, which the
	// platform reads from its Animator each frame they change.
	Nodes []ShaderNode
}

// ShaderNode binds lane Lane of a shader's uniforms to animated node
// Node.
type ShaderNode struct {
	Lane int
	Node uint32
}
