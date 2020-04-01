package interpreter

import (
	"github.com/raviqqe/hamt"

	"github.com/dapperlabs/cadence/runtime/ast"
	"github.com/dapperlabs/cadence/runtime/common"
	"github.com/dapperlabs/cadence/runtime/errors"
	"github.com/dapperlabs/cadence/runtime/sema"

	. "github.com/dapperlabs/cadence/runtime/trampoline"
)

// Invocation

type Invocation struct {
	Self               *CompositeValue
	Arguments          []Value
	ArgumentTypes      []sema.Type
	TypeParameterTypes map[*sema.TypeParameter]sema.Type
	LocationRange      LocationRange
	Interpreter        *Interpreter
}

// FunctionValue

type FunctionValue interface {
	Value
	isFunctionValue()
	Invoke(Invocation) Trampoline
}

// InterpretedFunctionValue

type InterpretedFunctionValue struct {
	Interpreter      *Interpreter
	ParameterList    *ast.ParameterList
	Type             *sema.FunctionType
	Activation       hamt.Map
	BeforeStatements []ast.Statement
	PreConditions    ast.Conditions
	Statements       []ast.Statement
	PostConditions   ast.Conditions
}

func (InterpretedFunctionValue) IsValue() {}

func (InterpretedFunctionValue) DynamicType(_ *Interpreter) DynamicType {
	return FunctionType{}
}

func (f InterpretedFunctionValue) Copy() Value {
	return f
}

func (InterpretedFunctionValue) GetOwner() *common.Address {
	// value is never owned
	return nil
}

func (InterpretedFunctionValue) SetOwner(owner *common.Address) {
	// NO-OP: value cannot be owned
}

func (InterpretedFunctionValue) isFunctionValue() {}

func (f InterpretedFunctionValue) Invoke(invocation Invocation) Trampoline {
	return f.Interpreter.invokeInterpretedFunction(f, invocation)
}

// HostFunctionValue

type HostFunction func(invocation Invocation) Trampoline

type HostFunctionValue struct {
	Function HostFunction
	Members  map[string]Value
}

func NewHostFunctionValue(
	function HostFunction,
) HostFunctionValue {
	return HostFunctionValue{
		Function: function,
	}
}

func (HostFunctionValue) IsValue() {}

func (HostFunctionValue) DynamicType(_ *Interpreter) DynamicType {
	return FunctionType{}
}

func (f HostFunctionValue) Copy() Value {
	return f
}

func (HostFunctionValue) GetOwner() *common.Address {
	// value is never owned
	return nil
}

func (HostFunctionValue) SetOwner(owner *common.Address) {
	// NO-OP: value cannot be owned
}

func (HostFunctionValue) isFunctionValue() {}

func (f HostFunctionValue) Invoke(invocation Invocation) Trampoline {
	return f.Function(invocation)
}

func (f HostFunctionValue) GetMember(interpreter *Interpreter, _ LocationRange, name string) Value {
	return f.Members[name]
}

func (f HostFunctionValue) SetMember(_ *Interpreter, _ LocationRange, _ string, _ Value) {
	panic(errors.NewUnreachableError())
}

// BoundFunctionValue

type BoundFunctionValue struct {
	Function FunctionValue
	Self     *CompositeValue
}

func (BoundFunctionValue) IsValue() {}

func (BoundFunctionValue) DynamicType(_ *Interpreter) DynamicType {
	return FunctionType{}
}

func (f BoundFunctionValue) Copy() Value {
	return f
}

func (BoundFunctionValue) GetOwner() *common.Address {
	// value is never owned
	return nil
}

func (BoundFunctionValue) SetOwner(owner *common.Address) {
	// NO-OP: value cannot be owned
}

func (BoundFunctionValue) isFunctionValue() {}

func (f BoundFunctionValue) Invoke(invocation Invocation) Trampoline {
	invocation.Self = f.Self
	return f.Function.Invoke(invocation)
}
