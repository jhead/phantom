package util

import (
	"reflect"
)

func MapFieldsToStruct(fields []interface{}, target interface{}) {
	numFields := len(fields)
	targetStruct := reflect.ValueOf(target).Elem()

	for i := 0; i < targetStruct.NumField() && i < numFields; i++ {
		field := targetStruct.Field(i)
		fieldValue := reflect.ValueOf(fields[i])
		field.Set(fieldValue)
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
