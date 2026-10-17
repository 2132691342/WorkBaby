// nil 切片归一化：HTTP 出口的统一兜底。
package pkg

import "reflect"

// maxFillDepth 递归上限：响应体是有限深度的契约对象，深到这个层数要么是环要么是设计错了。
const maxFillDepth = 8

// FillNilSlices 把 v（及其嵌套结构）里的 nil 切片换成空切片，返回可直接编码的值。
// 指针就地改，值类型返回副本；[]byte 与标量不进入递归。
func FillNilSlices(v any) any {
	if v == nil {
		return v
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Struct:
	default:
		return v
	}
	if rv.Kind() == reflect.Ptr {
		fillNil(rv, 0)
		return v
	}
	cp := reflect.New(rv.Type())
	cp.Elem().Set(rv)
	fillNil(cp.Elem(), 0)
	return cp.Elem().Interface()
}

func fillNil(rv reflect.Value, depth int) {
	if depth > maxFillDepth || !rv.IsValid() {
		return
	}
	switch rv.Kind() {
	case reflect.Interface:
		if !rv.IsNil() {
			fillNil(rv.Elem(), depth+1)
		}
	case reflect.Ptr:
		if !rv.IsNil() {
			fillNil(rv.Elem(), depth+1)
		}
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return // []byte 是整块数据，逐字节递归纯浪费
		}
		if rv.IsNil() {
			if rv.CanSet() {
				rv.Set(reflect.MakeSlice(rv.Type(), 0, 0))
			}
			return
		}
		for i := 0; i < rv.Len(); i++ {
			fillNil(rv.Index(i), depth+1)
		}
	case reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			fillNil(rv.Index(i), depth+1)
		}
	case reflect.Struct:
		for i := 0; i < rv.NumField(); i++ {
			fillNil(rv.Field(i), depth+1)
		}
	case reflect.Map:
		if rv.IsNil() {
			if rv.CanSet() {
				rv.Set(reflect.MakeMap(rv.Type()))
			}
			return
		}
		for _, k := range rv.MapKeys() {
			mv := rv.MapIndex(k)
			if mv.Kind() == reflect.Slice && mv.Type().Elem().Kind() == reflect.Uint8 {
				continue // []byte 值不递归
			}
			if mv.Kind() == reflect.Slice && mv.IsNil() {
				rv.SetMapIndex(k, reflect.MakeSlice(mv.Type(), 0, 0))
				continue
			}
			// map 的 value 不可寻址，只能改副本再整块塞回去
			cp := reflect.New(mv.Type())
			cp.Elem().Set(mv)
			fillNil(cp.Elem(), depth+1)
			rv.SetMapIndex(k, cp.Elem())
		}
	}
}
