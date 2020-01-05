package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type Foo struct {
	A string
	B string
	C string
}

type Bar struct {
	A string
	B int
	C bool
}

func TestToStruct(t *testing.T) {
	fields := []interface{}{"a", "b", "c"}
	foo := Foo{}

	MapFieldsToStruct(fields, &foo)

	assert.Equal(t, Foo{"a", "b", "c"}, foo)
}

func TestToStructWithExtra(t *testing.T) {
	fields := []interface{}{"a", "b", "c", "d"}
	foo := Foo{}

	MapFieldsToStruct(fields, &foo)

	assert.Equal(t, Foo{"a", "b", "c"}, foo)
}

func TestToStructWithMissing(t *testing.T) {
	fields := []interface{}{"a", "b"}
	foo := Foo{}

	MapFieldsToStruct(fields, &foo)

	assert.Equal(t, Foo{"a", "b", ""}, foo)
}

func TestToFields(t *testing.T) {
	fields := MapStructToFields(&Foo{"a", "b", "c"})
	expected := []interface{}{"a", "b", "c"}

	assert.Equal(t, expected, fields)
}

func TestToFieldsWithTypes(t *testing.T) {
	fields := MapStructToFields(&Bar{"a", 1, true})
	expected := []interface{}{"a", 1, true}

	assert.Equal(t, expected, fields)
}
