package jsonslice

import (
	"errors"
)

const (
	cOpErr = iota
	cOpInt
	cOpFloat
	cOpString
	cOpNode
	cOpOperator
	cOpBool
)

/*
var operatorFn map[byte]

func init() {
	setupOperatorMap()
}
*/

/*
  <operand> [ <operator> <operand> ] [ <compare> <operand> [ <operator> <operand> ] ]

  <filter> : <expression> [ <operator> <expression> ]   <--- .single
  <expression> : <operand> [ <operator> <operand> ]
  <compare> : /(==)|(!=)|(>)|(<)|(>=)|(<=)/
  <operand> : <number> | <string> | <bool> | <jsonpath>
  <number> : /-?[0-9]+(\.[0-9]*?)?/
  <string> : /"[^"]*"/
  <bool> : /(true)|(false)/
  <jsonpath> : /[@$].+/           <--- .exists
  <operator> : /[+-/*] | (>=,<=,==,!=,>,<)/
*/

type tFilter struct {
	toks []*tToken
}
type tToken struct {
	Operand  *tOperand
	Operator byte
}
type tOperand struct {
	Type     int
	NumInt   int64
	NumFloat float64
	Bool     bool
	Str      []byte
	Node     *tNode
}

var compares = [...]string{">=", "<=", "==", "!=", ">", "<"}
var compareOps = [...]byte{'g', 'l', 'E', 'N', 'G', 'L'}
var precedence = map[byte]int{'g': 1, 'l': 1, 'E': 1, 'N': 1, 'G': 1, 'L': 1, '+': 2, '-': 2, '*': 3, '/': 3}

type stack struct {
	s []*tToken
}

func reverse(s []*tToken) []*tToken {
	l := len(s)
	for i := 0; i < l/2; i++ {
		s[i], s[l-i-1] = s[l-i-1], s[i]
	}
	return s
}

func (s *stack) push(tok *tToken) {
	s.s = append(s.s, tok)
}
func (s *stack) pop() *tToken {
	l := len(s.s)
	if l == 0 {
		return nil
	}
	v := (s.s)[l-1]
	s.s = (s.s)[:l-1]
	return v
}
func (s *stack) peek() (tok *tToken) {
	l := len(s.s)
	if l == 0 {
		return nil
	}
	return (s.s)[l-1]
}
func (s *stack) get() []*tToken {
	return s.s
}

func readFilter(path []byte, i int, nod *tNode) (int, error) {
	l := len(path)

	// lexer
	tokens := make([]*tToken, 0)
	var tok *tToken
	var err error
	for i < l && path[i] != ')' {
		i, tok, err = nextToken(path, i)
		if err != nil {
			return i, err
		}
		if tok != nil {
			tokens = append(tokens, tok)
		}
	}

	//parser
	opStack := new(stack)
	result := new(stack)
	for t := len(tokens) - 1; t >= 0; t-- {
		op := tokens[t]
		if op.Operand != nil {
			result.push(op)
		} else {
			for {
				top := opStack.peek()
				if top != nil && precedence[top.Operator] >= precedence[op.Operator] {
					result.push(opStack.pop())
					continue
				}
				break
			}
			opStack.push(op)
		}
	}
	for {
		top := opStack.pop()
		if top == nil {
			break
		}
		result.push(top)
	}

	nod.Filter = &tFilter{toks: reverse(result.get())}

	return i, nil
}

// operand = (number, string, node), operator, compare
func nextToken(path []byte, i int) (int, *tToken, error) {
	var err error
	var tok *tToken
	l := len(path)
	for i < l && path[i] != ')' {
		i, err = skipSpaces(path, i)
		if err != nil {
			return 0, nil, err
		}
		if path[i] == ')' {
			return i, tok, nil
		}
		// number
		if path[i] >= '0' && path[i] <= '9' {
			return readNumber(path, i)
		}
		// string
		if path[i] == '"' {
			return readString(path, i)
		}
		// bool
		if path[i] == 't' || path[i] == 'f' {
			return readString(path, i)
		}
		// jsonpath node
		if path[i] == '@' || path[i] == '$' {
			nod, err := parsePath(path[i:])
			if err != nil {
				return 0, nil, err
			}
			for i < l && path[i] != ' ' && path[i] != ')' {
				i++
			}
			return i, &tToken{Operand: &tOperand{Type: cOpNode, Node: nod}}, nil
		}
		// operator
		if path[i] == '+' || path[i] == '-' || path[i] == '*' || path[i] == '/' {
			return i + 1, &tToken{Operator: path[i]}, nil
		}
		// compare
		if i >= l-1 {
			return i, nil, errors.New("unexpected end of token")
		}
		for ic, cmp := range compares {
			if string(path[i:i+len(cmp)]) == cmp {
				return i + len(cmp), &tToken{Operator: compareOps[ic]}, nil
			}
		}
		return 0, nil, errors.New("unknown token " + string(path[i]))
	}

	return i, tok, nil
}

func readNumber(path []byte, ii int) (int, *tToken, error) {
	typ := cOpErr
	i := ii
	sign := 1
	l := len(path)
	inum := 0
	fnum := 0
	base := 1
	num := &inum
	for i < l && (path[i] == '-' || path[i] == '.' || (path[i] >= '0' && path[i] <= '9')) {
		if typ == cOpErr {
			typ = cOpInt
		}
		ch := path[i]
		if ch == '-' {
			sign = -1
		} else if ch == '.' {
			if typ == cOpInt {
				typ = cOpFloat
				num = &fnum
			} else {
				return i, nil, errors.New("invalid floating point number")
			}
		} else {
			*num = (*num)*10 + int(ch-'0')
			base *= 10
		}
		i++
	}

	if typ == cOpInt {
		return i, &tToken{Operand: &tOperand{Type: typ, NumInt: int64(sign) * int64(inum)}}, nil
	}
	return i, &tToken{Operand: &tOperand{Type: typ, NumFloat: float64(sign) * (float64(inum) + float64(fnum)/float64(base))}}, nil
}

func readString(path []byte, i int) (int, *tToken, error) {
	i++ // quote
	s := i
	l := len(path)
	for i < l && path[i] != '"' {
		if path[i] == '\\' {
			i += 2
			continue
		}
		i++
	}
	if i == l {
		return i, nil, errors.New("unexpected end of string")
	}
	e := i
	i++ // unquote

	return i, &tToken{Operand: &tOperand{Type: cOpString, Str: path[s:e]}}, nil
}

func readBool(path []byte, i int) (int, *tToken, error) {
	s := i
	l := len(path)
	t, f := []byte("true\x00\x00"), []byte("false\x00")
	for i < l && (path[i] == t[i-s] || path[i] == f[i-s]) && (t[i-s] > 0 || f[i-s] > 0) {
		i++
	}
	if i == l || t[i-s] > 0 && f[i-s] > 0 {
		return i, nil, errors.New("invalid boolean value")
	}

	return i, &tToken{Operand: &tOperand{Type: cOpBool, Bool: path[s] == 't'}}, nil
}

// filterMatch
func filterMatch(input []byte, toks []*tToken) (bool, error) {
	if len(toks) == 0 {
		return false, errors.New("invalid filter")
	}
	op, err := evalToken(input, toks)
	if err != nil {
		return false, err
	}
	switch op.Type {
	case cOpBool:
		return op.Bool, nil
	case cOpFloat:
		return op.NumFloat > 0, nil
	case cOpInt:
		return op.NumInt > 0, nil
	case cOpString:
		return len(op.Str) > 0, nil
	case cOpNode:
		return op.Node.Exists, nil
	default:
		return false, nil
	}
}

func evalToken(input []byte, toks []*tToken) (*tOperand, error) {
	l := len(toks)
	tok := toks[0]
	if tok.Operand != nil {
		if tok.Operand.Type == cOpNode {
			val, err := getValue(input, tok.Operand.Node)
			if err != nil {
				tok.Operand.Type = cOpBool
				tok.Operand.Bool = false
				return tok.Operand, nil
			}
			return decodeValue(val, tok.Operand)
		}
		return tok.Operand, nil
	}
	if l < 2 {
		return nil, errors.New("not enough arguments")
	}
	switch tok.Operator {
	case '+':

	case '-':
	case '*':
	case '/':
	case 'g':
	case 'l':
	case 'E':
	case 'N':
	case 'G':
	case 'L':
	}
	return &tOperand{Type: cOpBool, Bool: true}, nil
}

func decodeValue(input []byte, op *tOperand) (*tOperand, error) {
	return op, nil
}
