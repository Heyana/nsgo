### Fixed

- Evaluate the shader math builtins — exponentials, logarithms, the inverse trigonometry, the hyperbolics, `sign`, `trunc`, `round`, `mod`, `distance`, `faceForward`, `refract`, `transpose`, `determinant`, the matrix constructors and the bit casts — on the software GPU device, and refuse a pipeline that calls a builtin it does not implement instead of drawing zeros (#2334 by @Laotree).
