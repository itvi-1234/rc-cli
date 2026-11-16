// Package validate checks a Runtime Conditions Profile against the extensions it declares.
//
// The validator is the same for every profiler and every language. It reads two kinds of YAML: the
// profile, and the definition of each extension the profile declares. It knows only what the core
// profile schema defines (a condition has a kind and an interface with a type, and may have a name
// and an optional flag). Everything else about a condition, the fields it may hold, their values and
// their shape, comes from the extension definitions, so an extension can give a field any shape and a
// new extension needs no change here.
package validate
