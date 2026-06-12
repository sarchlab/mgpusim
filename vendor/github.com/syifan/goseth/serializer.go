package goseth

import (
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
)

type serializeItem struct {
	id    uint64
	depth int
	item  reflect.Value
}

type serializer struct {
	root     any
	maxDepth int

	tempRoot   reflect.Value
	dict       []serializeItem
	nextDictID uint64
}

func (s *serializer) SetRoot(item any) {
	s.root = item
	s.tempRoot = reflect.ValueOf(item)
}

func (s *serializer) SetMaxDepth(depth int) {
	s.maxDepth = depth
}

func (s *serializer) SetEntryPoint(ep []string) error {
	v := reflect.ValueOf(s.root)
	for len(ep) > 0 {
		next := ep[0]
		ep = ep[1:]

		v = s.strip(v)

		switch v.Kind() {
		case reflect.Struct:
			v = v.FieldByName(next)
			if !v.IsValid() {
				return fmt.Errorf("field %s not found", next)
			}
		case reflect.Map:
			v = v.MapIndex(reflect.ValueOf(next))
			if !v.IsValid() {
				return fmt.Errorf("key %s not found", next)
			}
		case reflect.Slice:
			index, err := strconv.Atoi(next)
			if err != nil {
				return err
			}
			v = v.Index(index)
			if !v.IsValid() {
				return fmt.Errorf("index %d is not valid", index)
			}
		default:
			return fmt.Errorf("type %s is not supported", v.Type())
		}
	}

	s.tempRoot = v

	return nil
}

func (s *serializer) Serialize(writer io.Writer) error {
	s.dict = make([]serializeItem, 0)
	s.nextDictID = 0

	s.addToDict(s.tempRoot, 0)

	fmt.Fprintf(writer, `{"r":"0","dict":`)
	s.serializeDict(writer)
	fmt.Fprintf(writer, `}`)

	return nil
}

func (s *serializer) addToDict(v reflect.Value, depth int) uint64 {
	v = s.strip(v)
	s.dict = append(s.dict, serializeItem{
		id:    s.nextDictID,
		depth: depth,
		item:  v,
	})

	id := s.nextDictID

	s.nextDictID++

	return id
}

func (s *serializer) serializeDict(writer io.Writer) {
	fmt.Fprintf(writer, `{`)

	count := 0
	for len(s.dict) > 0 {
		if count > 0 {
			fmt.Fprintf(writer, `,`)
		}
		count++

		s.serializeOneDictItem(writer)
	}

	fmt.Fprintf(writer, `}`)
}

func (s *serializer) serializeOneDictItem(writer io.Writer) {
	item := s.dict[0]
	s.dict = s.dict[1:]

	k := item.id
	v := item.item
	v = s.strip(v)

	if s.isZero(v) {
		fmt.Fprintf(writer, `"%d":{"k":0,"t":"null","v":null}`, k)
	} else {
		fmt.Fprintf(writer, `"%d":{"k":%d,"t":"%s"`,
			k, v.Kind(), s.typeString(v))
		if s.needSerializeValue(item) {
			fmt.Fprint(writer, `,"v":`)
			s.serializeValue(writer, v, item.depth)
		}
		if s.needSerializeLen(item) {
			fmt.Fprintf(writer, `,"l":%d`, v.Len())
		}
		fmt.Fprintf(writer, `}`)
	}
}

func (s *serializer) needSerializeValue(item serializeItem) bool {
	if s.maxDepth < 0 {
		return true
	}

	if item.depth < s.maxDepth {
		return true
	}

	v := s.strip(item.item)
	switch v.Kind() {
	case reflect.Struct, reflect.Map, reflect.Slice:
		return false
	}

	return true
}

func (s *serializer) needSerializeLen(item serializeItem) bool {
	v := s.strip(item.item)
	switch v.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Chan:
		return true
	}

	return false
}

func (s *serializer) isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Invalid:
		return true
	case reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func (s *serializer) strip(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		return s.strip(v.Elem())
	default:
		return v
	}
}

func (s *serializer) serializeValue(
	writer io.Writer,
	v reflect.Value,
	depth int,
) {
	switch v.Kind() {
	case reflect.Bool:
		fmt.Fprintf(writer, `%t`, v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		fmt.Fprintf(writer, `%d`, v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		fmt.Fprintf(writer, `%d`, v.Uint())
	case reflect.Float32, reflect.Float64:
		fmt.Fprintf(writer, `%f`, v.Float())
	case reflect.String:
		fmt.Fprintf(writer, `"%s"`, v.String())
	case reflect.Chan:
		s.serializeChan(writer, v, depth)
	case reflect.Slice:
		s.serializeSlice(writer, v, depth)
	case reflect.Map:
		s.serializeMap(writer, v, depth)
	case reflect.Struct:
		s.serializeStruct(writer, v, depth)
	default:
		panic(fmt.Sprintf("kind %s not supported", v.Kind().String()))
	}
}

func (s *serializer) serializeChan(
	writer io.Writer,
	v reflect.Value,
	_ int,
) {
	fmt.Fprintf(writer, `{"k":18,"t":"%s","l":%d}`,
		s.typeString(v), v.Len())
}

func (s *serializer) serializeMap(
	writer io.Writer,
	v reflect.Value,
	depth int,
) {
	fmt.Fprintf(writer, `{`)

	for i, key := range v.MapKeys() {
		if i > 0 {
			fmt.Fprint(writer, `,`)
		}

		keyID := s.addToDict(key, depth+1)
		valueID := s.addToDict(v.MapIndex(key), depth+1)
		fmt.Fprintf(writer, `"%d":"%d"`, keyID, valueID)
	}

	fmt.Fprintf(writer, `}`)
}

func (s *serializer) serializeSlice(
	writer io.Writer,
	v reflect.Value,
	depth int,
) {
	fmt.Fprintf(writer, `[`)
	for i := 0; i < v.Len(); i++ {
		if i > 0 {
			fmt.Fprint(writer, `,`)
		}

		id := s.addToDict(v.Index(i), depth+1)
		fmt.Fprintf(writer, `"%d"`, id)
	}
	fmt.Fprintf(writer, `]`)
}

func (s *serializer) serializeStruct(
	writer io.Writer,
	v reflect.Value,
	depth int,
) {
	fmt.Fprintf(writer, `{`)

	for i := 0; i < v.NumField(); i++ {
		if i > 0 {
			fmt.Fprint(writer, `,`)
		}

		field := v.Field(i)
		fieldID := s.addToDict(field, depth+1)

		fmt.Fprintf(writer, `"%s":"%d"`, v.Type().Field(i).Name, fieldID)
	}

	fmt.Fprintf(writer, `}`)
}

func (s *serializer) typeString(value reflect.Value) string {
	name := value.Type().String()
	pktPath := value.Type().PkgPath()

	if pktPath == "" {
		return name
	}

	tokens := strings.Split(pktPath, "/")
	tokens = tokens[0 : len(tokens)-1]
	pktPath = strings.Join(tokens, "/")

	return fmt.Sprintf("%s/%s", pktPath, name)
}
