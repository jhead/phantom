package util

import (
	"reflect"
)

// Takes an arbitrary struct reference and apples all the values from
// an array to its fields.
func MapFieldsToStruct(fields []interface{}, target interface{}) {
	numFields := len(fields)
	targetStruct := reflect.ValueOf(target).Elem()

	// Iterate over the fields in the struct and array using the same index
	for i := 0; i < targetStruct.NumField() && i < numFields; i++ {
		field := targetStruct.Field(i)
		fieldValue := reflect.ValueOf(fields[i])

		// Make sure the types match first, otherwise skip
		if field.Type() == fieldValue.Type() {
			// Put the value from the array into the struct
			field.Set(fieldValue)
		}
	}
}

func MapStructToFields(source interface{}) []interface{} {
	var fields []interface{}

	sourceStruct := reflect.ValueOf(source).Elem()

	for i := 0; i < sourceStruct.NumField(); i++ {
		field := sourceStruct.Field(i).Interface()
		fields = append(fields, field)
	}

	return fields
}
