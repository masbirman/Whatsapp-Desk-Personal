package main

import _ "embed"

// domAdapterSource is injected before feature modules so all DOM-dependent
// code can use the same selector registry and ambiguity rules.
//
//go:embed dom_adapter.js
var domAdapterSource string
