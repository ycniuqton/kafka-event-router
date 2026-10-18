// Package recipes implements the native recipe steps, one file per kind (header
// operations share header_ops.go).
package recipes

import "github.com/eventrouter/router/pkg/engine/step"

// Default returns a StepCompilerManager with every recipe, map, js and the library placeholder.
func Default() *step.StepCompilerManager {
	return step.NewStepCompilerManager(
		Rename{}, Remove{}, KeepOnly{}, Copy{}, Move{},
		Set{}, DefaultIfMissing{}, SetKey{},
		Mask{}, Hash{}, ConvertType{}, DropIf{},
		step.JSCompiler{}, Library{},
		Flatten{}, RedactPattern{}, FormatDate{}, Round{}, StringCase{},
		Lookup{}, IfElse{}, SplitArray{}, Wrap{}, Unwrap{},
		HeaderAdd{}, HeaderRemove{}, HeaderRename(), HeaderCopyToPayload(), HeaderCopyFromPayload(),
		step.MapperCompiler{},
	)
}
