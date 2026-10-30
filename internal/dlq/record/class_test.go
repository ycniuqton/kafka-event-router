package record

import (
	"testing"
	"time"
)

func TestEveryErrorTypeHasAClass(t *testing.T) {
	for _, et := range append(append([]ErrorType{}, ErrorTypes...), ErrorUnknown) {
		if _, ok := ErrorClasses[et]; !ok {
			t.Errorf("%s has no class", et)
		}
		if !et.Class().Valid() {
			t.Errorf("%s class %q invalid", et, et.Class())
		}
	}
}

func TestClassificationTable(t *testing.T) {
	cases := map[ErrorType]ErrorClass{
		ErrorStep: ClassData, ErrorDecode: ClassData, ErrorOutputInvalid: ClassData,
		ErrorTimeout: ClassTransient, ErrorLookupUnavailable: ClassTransient, ErrorProduceRetriable: ClassTransient,
		ErrorDependencyUnavailable: ClassTransient, ErrorInternal: ClassRouter, ErrorSandboxCrash: ClassRouter,
		ErrorEncode: ClassRouter, ErrorProduce: ClassRouter, ErrorConfig: ClassRouter, ErrorUnknown: ClassRouter,
		ErrorType("brand_new"): ClassRouter,
	}
	for et, want := range cases {
		if got := et.Class(); got != want {
			t.Errorf("%s: got %s want %s", et, got, want)
		}
	}
}

func TestTypesOfPartitionsTheTypes(t *testing.T) {
	seen := map[ErrorType]bool{}
	for _, c := range ErrorClassList {
		for _, et := range TypesOf(c) {
			if seen[et] {
				t.Fatalf("%s listed twice", et)
			}
			seen[et] = true
		}
	}
	if len(seen) != len(ErrorTypes)+1 {
		t.Fatalf("got %d types, want %d", len(seen), len(ErrorTypes)+1)
	}
	if ErrorClass("nope").Valid() {
		t.Fatal("unknown class valid")
	}
}

func TestCodecKeepsRetryAttempts(t *testing.T) {
	r := Record{Failure: Failure{RouteID: "r1", VersionNo: 2, ErrorType: ErrorLookupUnavailable, Message: "m",
		FailedAt: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC), Attempts: 5}}
	got, err := Codec{}.Decode(Codec{}.Encode(r))
	if err != nil || got.Failure.Attempts != 5 || got.Failure.ErrorType != ErrorLookupUnavailable {
		t.Fatalf("decode %+v %v", got.Failure, err)
	}
	r.Failure.Attempts = 0
	for _, h := range (Codec{}).Encode(r).Headers {
		if h.Key == string(HeaderRetryAttempts) {
			t.Fatal("attempts header written for 0 attempts")
		}
	}
}
