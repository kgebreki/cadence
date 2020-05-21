/*
 * Cadence - The resource-oriented smart contract programming language
 *
 * Copyright 2019-2020 Dapper Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package parser2

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/onflow/cadence/runtime/ast"
	"github.com/onflow/cadence/runtime/common"
	"github.com/onflow/cadence/runtime/errors"
	"github.com/onflow/cadence/runtime/parser2/lexer"
)

func parseDeclarations(p *parser, endTokenType lexer.TokenType) (declarations []ast.Declaration) {
	for {
		p.skipSpaceAndComments(true)

		switch p.current.Type {
		case lexer.TokenSemicolon:
			p.next()
			continue

		case endTokenType, lexer.TokenEOF:
			return

		default:
			declaration := parseDeclaration(p)
			if declaration == nil {
				return
			}

			declarations = append(declarations, declaration)
		}
	}
}

func parseDeclaration(p *parser) ast.Declaration {

	access := ast.AccessNotSpecified
	var accessPos *ast.Position

	for {
		p.skipSpaceAndComments(true)

		switch p.current.Type {
		case lexer.TokenIdentifier:
			switch p.current.Value {
			case keywordLet, keywordVar:
				return parseVariableDeclaration(p, access, accessPos)

			case keywordFun:
				return parseFunctionDeclaration(p, access, accessPos)

			case keywordImport:
				return parseImportDeclaration(p)

			case keywordEvent:
				return parseEventDeclaration(p, access, accessPos)

			case keywordPriv, keywordPub, keywordAccess:
				if access != ast.AccessNotSpecified {
					panic(fmt.Errorf("unexpected access modifier"))
				}
				pos := p.current.StartPos
				accessPos = &pos
				access = parseAccess(p)
				continue
			}
		}

		return nil
	}
}

// parseAccess parses an access modifier
//
//    access
//        : 'priv'
//        | 'pub' ( '(' 'set' ')' )?
//        | 'access' '(' ( 'self' | 'contract' | 'account' | 'all' ) ')'
//        ;
//
func parseAccess(p *parser) ast.Access {

	switch p.current.Value {
	case keywordPriv:
		p.next()
		return ast.AccessPrivate

	case keywordPub:
		p.next()
		p.skipSpaceAndComments(true)
		if !p.current.Is(lexer.TokenParenOpen) {
			return ast.AccessPublic
		}

		p.next()
		p.skipSpaceAndComments(true)

		if !p.current.Is(lexer.TokenIdentifier) {
			panic(fmt.Errorf(
				"expected keyword %q, got %s",
				keywordSet,
				p.current.Type,
			))
		}
		if p.current.Value != keywordSet {
			panic(fmt.Errorf(
				"expected keyword %q, got %q",
				keywordSet,
				p.current.Value,
			))
		}

		p.next()
		p.skipSpaceAndComments(true)

		p.mustOne(lexer.TokenParenClose)

		return ast.AccessPublicSettable

	case keywordAccess:
		p.next()
		p.skipSpaceAndComments(true)

		p.mustOne(lexer.TokenParenOpen)

		p.skipSpaceAndComments(true)

		if !p.current.Is(lexer.TokenIdentifier) {
			panic(fmt.Errorf(
				"expected keyword %q, %q, %q, or %q, got %s",
				keywordAll,
				keywordAccount,
				keywordContract,
				keywordSelf,
				p.current.Type,
			))
		}

		var access ast.Access

		switch p.current.Value {
		case keywordAll:
			access = ast.AccessPublic

		case keywordAccount:
			access = ast.AccessAccount

		case keywordContract:
			access = ast.AccessContract

		case keywordSelf:
			access = ast.AccessPrivate

		default:
			panic(fmt.Errorf(
				"expected keyword %q, %q, %q, or %q, got %q",
				keywordAll,
				keywordAccount,
				keywordContract,
				keywordSelf,
				p.current.Value,
			))
		}

		p.next()
		p.skipSpaceAndComments(true)

		p.mustOne(lexer.TokenParenClose)

		return access

	default:
		panic(errors.NewUnreachableError())
	}
}

// parseVariableDeclaration parses a variable declaration.
//
//     variableKind : 'var' | 'let' ;
//
//     variableDeclaration :
//         variableKind identifier ( ':' typeAnnotation )?
//         transfer expression
//         ( transfer expression )?
//
func parseVariableDeclaration(p *parser, access ast.Access, accessPos *ast.Position) *ast.VariableDeclaration {

	startPos := p.current.StartPos
	if accessPos != nil {
		startPos = *accessPos
	}

	isLet := p.current.Value == keywordLet

	// Skip the `let` or `var` keyword
	p.next()

	p.skipSpaceAndComments(true)
	if !p.current.Is(lexer.TokenIdentifier) {
		panic(fmt.Errorf(
			"expected identifier after start of variable declaration, got %s",
			p.current.Type,
		))
	}

	identifier := tokenToIdentifier(p.current)

	p.next()
	p.skipSpaceAndComments(true)

	var typeAnnotation *ast.TypeAnnotation

	if p.current.Is(lexer.TokenColon) {
		p.next()
		p.skipSpaceAndComments(true)

		typeAnnotation = parseTypeAnnotation(p)
	}

	p.skipSpaceAndComments(true)
	transfer := parseTransfer(p)
	if transfer == nil {
		panic(fmt.Errorf("expected transfer"))
	}

	value := parseExpression(p, lowestBindingPower)

	p.skipSpaceAndComments(true)

	secondTransfer := parseTransfer(p)
	var secondValue ast.Expression
	if secondTransfer != nil {
		secondValue = parseExpression(p, lowestBindingPower)
	}

	return &ast.VariableDeclaration{
		Access:         access,
		IsConstant:     isLet,
		Identifier:     identifier,
		TypeAnnotation: typeAnnotation,
		Value:          value,
		Transfer:       transfer,
		StartPos:       startPos,
		SecondTransfer: secondTransfer,
		SecondValue:    secondValue,
	}
}

// parseTransfer parses a transfer.
//
//    transfer : '=' | '<-' | '<-!'
//
func parseTransfer(p *parser) *ast.Transfer {
	var operation ast.TransferOperation

	switch p.current.Type {
	case lexer.TokenEqual:
		operation = ast.TransferOperationCopy

	case lexer.TokenLeftArrow:
		operation = ast.TransferOperationMove

	case lexer.TokenLeftArrowExclamation:
		operation = ast.TransferOperationMoveForced
	}

	if operation == ast.TransferOperationUnknown {
		return nil
	}

	pos := p.current.StartPos

	p.next()

	return &ast.Transfer{
		Operation: operation,
		Pos:       pos,
	}
}

// parseImportDeclaration parses an import declaration
//
//   importDeclaration :
//       'import'
//       ( identifier (',' identifier)* 'from' )?
//       ( string | hexadecimalLiteral | identifier )
//
func parseImportDeclaration(p *parser) *ast.ImportDeclaration {

	startPosition := p.current.StartPos

	var identifiers []ast.Identifier

	var location ast.Location
	var locationPos ast.Position
	var endPos ast.Position

	parseStringOrAddressLocation := func() {
		locationPos = p.current.StartPos
		endPos = p.current.EndPos

		switch p.current.Type {
		case lexer.TokenString:
			parsedString, errs := parseStringLiteral(p.current.Value.(string))
			p.report(errs...)
			location = ast.StringLocation(parsedString)

		case lexer.TokenHexadecimalLiteral:
			location = parseHexadecimalLocation(p.current.Value.(string))

		default:
			panic(errors.NewUnreachableError())
		}

		p.next()
	}

	setIdentifierLocation := func(identifier ast.Identifier) {
		// TODO: create IdentifierLocation once https://github.com/onflow/cadence/pull/55 is merged
		//location = ast.IdentifierLocation(identifier.Identifier)
		locationPos = identifier.Pos
		endPos = identifier.EndPosition()
	}

	parseLocation := func() {
		switch p.current.Type {
		case lexer.TokenString, lexer.TokenHexadecimalLiteral:
			parseStringOrAddressLocation()

		// TODO: enable once https://github.com/onflow/cadence/pull/55 is merged
		//case lexer.TokenIdentifier:
		//	identifier := tokenToIdentifier(p.current)
		//	setIdentifierLocation(identifier)
		//  p.next()

		default:
			panic(fmt.Errorf(
				"unexpected token in import declaration: got %s, expected string, address, or identifier",
				p.current.Type,
			))
		}
	}

	parseMoreIdentifiers := func() {
		expectCommaOrFrom := false

		atEnd := false
		for !atEnd {
			p.next()
			p.skipSpaceAndComments(true)

			switch p.current.Type {
			case lexer.TokenComma:
				if !expectCommaOrFrom {
					panic(fmt.Errorf(
						"expected %s or keyword %q, got %s",
						lexer.TokenIdentifier,
						keywordFrom,
						p.current.Type,
					))
				}
				expectCommaOrFrom = false

			case lexer.TokenIdentifier:

				if p.current.Value == keywordFrom {

					if !expectCommaOrFrom {
						panic(fmt.Errorf(
							"expected %s, got keyword %q",
							lexer.TokenIdentifier,
							p.current.Value,
						))
					}

					atEnd = true

					p.next()
					p.skipSpaceAndComments(true)

					parseLocation()
				} else {
					identifier := tokenToIdentifier(p.current)
					identifiers = append(identifiers, identifier)

					expectCommaOrFrom = true
				}

			case lexer.TokenEOF:
				panic(fmt.Errorf(
					"unexpected end in import declaration: expected %s or %s",
					lexer.TokenIdentifier,
					lexer.TokenComma,
				))

			default:
				panic(fmt.Errorf(
					"unexpected token in import declaration: got %s, expected keyword %q or %s",
					p.current.Type,
					keywordFrom,
					lexer.TokenComma,
				))
			}
		}
	}

	maybeParseFromIdentifier := func(identifier ast.Identifier) {
		// The current identifier is maybe the `from` keyword,
		// in which case the given (previous) identifier was
		// an imported identifier and not the import location.
		//
		// If it is not the `from` keyword,
		// the given (previous) identifier is the import location.

		if p.current.Value == keywordFrom {
			identifiers = append(identifiers, identifier)

			p.next()
			p.skipSpaceAndComments(true)

			parseLocation()

		} else {
			// TODO: enable once https://github.com/onflow/cadence/pull/55 is merged
			//setIdentifierLocation(identifier)

			// TODO: remove once https://github.com/onflow/cadence/pull/55 is merged
			panic(fmt.Errorf(
				"unexpected identifier in import declaration: got %q, expected %q",
				p.current.Value,
				keywordFrom,
			))
		}
	}

	// Skip the `import` keyword
	p.next()
	p.skipSpaceAndComments(true)

	switch p.current.Type {
	case lexer.TokenString, lexer.TokenHexadecimalLiteral:
		parseStringOrAddressLocation()

	case lexer.TokenIdentifier:
		identifier := tokenToIdentifier(p.current)

		p.next()
		p.skipSpaceAndComments(true)

		switch p.current.Type {
		case lexer.TokenComma:
			// The previous identifier is an imported identifier,
			// not the import location
			identifiers = append(identifiers, identifier)
			parseMoreIdentifiers()

		case lexer.TokenIdentifier:
			maybeParseFromIdentifier(identifier)

		case lexer.TokenEOF:
			// The previous identifier is the identifier location
			setIdentifierLocation(identifier)

		default:
			panic(fmt.Errorf(
				"unexpected token in import declaration: got %s, expected keyword %q or %s",
				p.current.Type,
				keywordFrom,
				lexer.TokenComma,
			))
		}

	case lexer.TokenEOF:
		panic(fmt.Errorf("unexpected end in import declaration: expected string, address, or identifier"))

	default:
		panic(fmt.Errorf(
			"unexpected token in import declaration: got %s, expected string, address, or identifier",
			p.current.Type,
		))
	}

	return &ast.ImportDeclaration{
		Identifiers: identifiers,
		Location:    location,
		Range: ast.Range{
			StartPos: startPosition,
			EndPos:   endPos,
		},
		LocationPos: locationPos,
	}
}

func parseHexadecimalLocation(literal string) ast.AddressLocation {
	bytes := []byte(strings.Replace(literal[2:], "_", "", -1))

	length := len(bytes)
	if length%2 == 1 {
		bytes = append([]byte{'0'}, bytes...)
		length++
	}

	address := make([]byte, hex.DecodedLen(length))
	_, err := hex.Decode(address, bytes)
	if err != nil {
		// unreachable, hex literal should always be valid
		panic(err)
	}

	return address
}

// parseEventDeclaration parses an event declaration.
//
//    eventDeclaration : 'event' identifier parameterList
//
func parseEventDeclaration(p *parser, access ast.Access, accessPos *ast.Position) *ast.CompositeDeclaration {

	startPos := p.current.StartPos
	if accessPos != nil {
		startPos = *accessPos
	}

	// Skip the `event` keyword
	p.next()

	p.skipSpaceAndComments(true)
	if !p.current.Is(lexer.TokenIdentifier) {
		panic(fmt.Errorf(
			"expected identifier after start of event declaration, got %s",
			p.current.Type,
		))
	}

	identifier := ast.Identifier{
		Identifier: p.current.Value.(string),
		Pos:        p.current.StartPos,
	}

	p.next()

	parameterList := parseParameterList(p)

	initializer :=
		&ast.SpecialFunctionDeclaration{
			DeclarationKind: common.DeclarationKindInitializer,
			FunctionDeclaration: &ast.FunctionDeclaration{
				ParameterList: parameterList,
				StartPos:      parameterList.StartPos,
			},
		}

	return &ast.CompositeDeclaration{
		Access:        access,
		CompositeKind: common.CompositeKindEvent,
		Identifier:    identifier,
		Members: &ast.Members{
			SpecialFunctions: []*ast.SpecialFunctionDeclaration{
				initializer,
			},
		},
		Range: ast.Range{
			StartPos: startPos,
			EndPos:   parameterList.EndPos,
		},
	}
}
