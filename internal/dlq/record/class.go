package record

// ErrorClass says whose fault a dead letter is: bad input data, a router bug, or a transient condition
// that heals by retrying. It is a pure function of the error type (no header), so old records classify too.
type ErrorClass string

const (
	ClassData      ErrorClass = "data"
	ClassRouter    ErrorClass = "router"
	ClassTransient ErrorClass = "transient"
)

// ErrorClassList lists the classes in display order.
var ErrorClassList = []ErrorClass{ClassData, ClassRouter, ClassTransient}

// Valid reports whether c is a known class.
func (c ErrorClass) Valid() bool {
	for _, k := range ErrorClassList {
		if k == c {
			return true
		}
	}
	return false
}

// ErrorClasses is the classification table of every error type (design 00001 §2).
var ErrorClasses = map[ErrorType]ErrorClass{
	ErrorStep:                  ClassData,
	ErrorDecode:                ClassData,
	ErrorOutputInvalid:         ClassData,
	ErrorTimeout:               ClassTransient,
	ErrorLookupUnavailable:     ClassTransient,
	ErrorDependencyUnavailable: ClassTransient,
	ErrorProduceRetriable:      ClassTransient,
	ErrorInternal:              ClassRouter,
	ErrorSandboxCrash:          ClassRouter,
	ErrorEncode:                ClassRouter,
	ErrorProduce:               ClassRouter,
	ErrorConfig:                ClassRouter,
	ErrorUnknown:               ClassRouter,
}

// Class returns the class of t; an unmapped type is a router bug (fail loud, never hide our own errors).
func (t ErrorType) Class() ErrorClass {
	if c, ok := ErrorClasses[t]; ok {
		return c
	}
	return ClassRouter
}

// TypesOf returns the error types of class c in ErrorTypes order (plus unknown for the router class).
func TypesOf(c ErrorClass) []ErrorType {
	out := []ErrorType{}
	for _, t := range append(append([]ErrorType{}, ErrorTypes...), ErrorUnknown) {
		if t.Class() == c {
			out = append(out, t)
		}
	}
	return out
}
