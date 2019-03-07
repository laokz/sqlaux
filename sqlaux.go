// Package sqlaux 实现了几个快速、轻量的SQL辅助函数，目的是更加方便地从数据
// 库接收查询结果、向数据库插入更新数据，辅助程序员构建自己的InsertDelete
// UpdateSelect包。sqlaux用少量的空间成本换取最少的反射包调用，以此确保函数
// 的时间性能。
//
// sqlaux假设应用程序的数据结构，与数据库表有着一一对应的关系；但允许一个数
// 据结构对应多个同构异名的表（表列定义完全相同，表名不同），如历史记录等。
// 为避免数据库定义大小写歧义，sqlaux还将列名全部转为小写再处理；建立映射关
// 系时也不包含表名，因为查询结果通常并不包含表名信息，程序员需保证用正确的
// 数据结构读写正确的数据库表。
//
// sqlaux支持两种映射：
//	1. 小写(列名) != 小写(字段名)时的名称映射。这是必需的映射。
//		sqlaux 通过struct tag识别字段、列名对应关系，默认将所有导出字段名
//		（含除time.Time外的嵌套结构成员）小写，作为与其对应的数据库表列名。
//	2. 字段类型不能直接用于数据库读写时的类型映射。这是可选的映射。
//		通常这时字段应使用自定义类型，并且实现sql.Scanner和/或driver.Valuer
//		接口，这不需要作映射。但对于切片等Go原生类型，直接使用自定义类型会带
//		来很多类型转换问题。这两个接口仅用于数据库读写，因此sqlaux提供类型映
//		射方法，可以使得程序继续使用Go原生类型，而在sqlaux内部使用与其等价的
//		自定义类型进行数据库读写。
package sqlaux

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"unicode"
	"unsafe"
)

// entryT 表示实际映射项信息。name 用于field-->column的映射，表示列名，当键
// 为"struct名"时，name为该结构所有映射字段名的切片，当entryT用于，确定具体
// 接收字段地址时，name借指字段所在结构在接收结构切片中的索引；offset 表示字
// 段相对最外层struct的全局偏移；typ为字段类型，或其等价的实现了sql.Scanner/
// driver.Valuer接口的自定义类型。offset、typ在两个映射中是重复的。???
type entryT struct {
	name   interface{}
	offset uintptr
	typ    reflect.Type
}

// mapping 为Go数据结构与数据库表的映射。key 分为三种情况：
//	● "1.struct名.column名"，表示column-->field的映射，用于Scan()
//	● "0.struct名.field名"，表示field-->column的映射，用于Buildstr()
//	● "struct名"，表示该结构的映射已建立
var mapping = make(map[string]entryT)

// isinit 检查映射初始化函数是否在init()中调用，以防止出现竞争条件。
func isinit() bool {
	var pc [10]uintptr
	n := runtime.Callers(2, pc[:])
	fs := runtime.CallersFrames(pc[:n])
	var f runtime.Frame
	b := true
	for b {
		f, b = fs.Next()
		if strings.Contains(f.Function, ".init.") { // xxx.init.N
			return true
		}
	}
	return false
}

// 以下三个导出变量为struct tag，用于sqlaux识别结构字段所对应的数据库列名。
// Tag标签名；Key列名键；Op键值分隔符。如：`db:"col=xxx yyy=zzz"`
var (
	Tag = "db"
	Key = "col"
	Op  = "="
)

// MapStruct 为Go数据结构与数据库表建立名称映射。调用者需在init()中，对每一
// 个关联数据库的结构调用此函数进行显式映射。
// stru为需要映射的数据结构，以变量值的形式作参数，可以取零值。
func MapStruct(stru ...interface{}) error {
	// check caller is init(), ensure no race condition
	if !isinit() {
		return fmt.Errorf("MapStruct: must be called in init()")
	}

	for _, d := range stru {
		v := reflect.Indirect(reflect.ValueOf(d))
		s := v.Type().Name()
		if s == "" || v.Kind() != reflect.Struct {
			return fmt.Errorf("MapStruct: invalid struct %q", v.Type())
		}
		if _, ok := mapping[s]; ok {
			return fmt.Errorf("MapStruct: %q already mapped", v.Type())
		}
		fs, err := initmap(s, v, 0)
		if err != nil {
			return fmt.Errorf("MapStruct: %v", err)
		}
		mapping[s] = entryT{name: fs} // mark this struct as initiated
	}

	return nil
}

// initmap 递归遍历结构v，为所有导出字段创建映射项。s为完整结构名（可能为嵌
// 套结构），b为结构相对于最外层结构的全局偏移量，返回的切片为字段名。
func initmap(s string, v reflect.Value, b uintptr) ([]string, error) {
	dot := strings.Index(s, ".") // for diff the most outer struct name
	t := v.Type()
	fs := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tt := t.Field(i)
		if !unicode.IsUpper([]rune(tt.Name)[0]) { // ignore non-exported
			continue
		}
		col := strings.ToLower(tt.Name) // record its default column name
		nt := tt.Type                   // record its type
		if ntt, ok := typemap[nt]; ok { // use mapped type if possible
			nt = ntt
		}
		got := false // record if found tagged column name
		tags := tt.Tag.Get(Tag)
		if tags != "" {
			vs := strings.Fields(tags)
			for _, v := range vs {
				fc := strings.Split(v, Op)
				if fc[0] == Key {
					col = fc[1]
					got = true
					break
				}
			}
		}
		if !got && tt.Type.Kind() == reflect.Struct && // recursive struct
			tt.Type.String() != "time.Time" { // except "time.Time"
			ffs, err := initmap(s+"."+tt.Name, v.Field(i), b+tt.Offset)
			if err != nil {
				return nil, err
			}
			fs = append(fs, ffs...)
		} else {
			if col == "" || strings.ToLower(col) != col {
				return nil, fmt.Errorf("%s.%s bad tagged 'col'", s, tt.Name)
			}
			mapping["0."+s+"."+tt.Name] = entryT{col, b + tt.Offset, nt}
			sss := "1." // "1.the-most-outer-struct.column"
			if dot == -1 {
				sss += s + "." + col
				fs = append(fs, tt.Name) // without the most outer struct
			} else {
				sss += s[:dot+1] + col
				fs = append(fs, s[dot+1:]+"."+tt.Name)
			}
			if _, ok := mapping[sss]; ok { // column maybe wrong duplicate
				return nil, fmt.Errorf("%q duplicate column map %q", s, col)
			}
			mapping[sss] = entryT{nil, b + tt.Offset, nt}
		}
	}
	return fs, nil
}

// typemap 为字段类型到其等价的实现了sql.Scanner/driver.Valuer接口的自定义类
// 型的映射。这实际上是一个临时变量，初始化过程结束后，该变量就不再使用。???
var typemap = make(map[reflect.Type]reflect.Type)

// MapType 为Go数据结构与数据库表建立类型映射。调用者需在init()中，针对每一
// 个在程序中使用Go原生类型，而在数据库读写时使用自定义类型的情况，调用此函
// 数进行显式映射。orig、self分别为原生类型和自定义类型值，可以用零值。
// sql.Scanner接收器为指针型，driver.Valuer接收器为值型，sqlaux约定这里的参
// 数统一用T而不用*T。参见包文档和README。
func MapType(orig, self interface{}) error {
	// check caller is init(), ensure no race condition
	if !isinit() {
		return fmt.Errorf("MapType: must be called in init()")
	}

	// check if already initiated
	ov := reflect.TypeOf(orig)
	sv := reflect.TypeOf(self)
	if _, ok := typemap[ov]; ok {
		return fmt.Errorf("MapType: type %q already mapped", ov)
	}

	// check if the 2 types deep equal
	if !ov.ConvertibleTo(sv) {
		return fmt.Errorf("MapType: %q not convertible to %q", ov, sv)
	}

	// map and update mapping
	typemap[ov] = sv
	for k, v := range mapping {
		if v.typ == ov {
			v.typ = sv
			mapping[k] = v
		}
	}
	return nil
}

// Scan 从已执行完查询的rows中，接收当前结果集（即一个单独的SELECT）的所有结
// 果，覆盖写入dest。接收后Scan 不主动关闭rows。
//
// 约定：
//	● 每一个dest的类型形如*[]*struct。
//	● SELECT选择列时逐表罗列，表间可选地用''空列分隔。当两表“交界”处有重名列
//		时，默认sqlaux将其视为前一个表的列，用空列区隔可避免重名歧义。
func Scan(rows *sql.Rows, dest ...interface{}) error {
	l := len(dest)
	if l == 0 {
		return fmt.Errorf("Scan: no dest argument")
	}

	// prepare receiver variable
	typ := make([]reflect.Type, l)  // type of every dest
	rsa := make([]reflect.Value, l) // []*struct, for accumulating results
	for i, d := range dest {
		t := reflect.TypeOf(d)          // *[]*struct
		typ[i] = t.Elem().Elem().Elem() // struct
		if !strings.HasPrefix(t.String(), "*[]*") ||
			typ[i].Kind() != reflect.Struct {
			return fmt.Errorf("Scan: dest[%d] not like *[]*struct", i)
		}
		rsa[i] = reflect.MakeSlice(t.Elem(), 0, 0) // []*struct
	}
	ref, err := scanField(rows, typ) // calculate dest fields reference
	if err != nil {
		return fmt.Errorf("Scan: %v", err)
	}
	var null = new(string) // for NULL column '', cannot be nil

	// receive all results
	tmp := make([]reflect.Value, l)      // new struct variable for a scan
	ptr := make([]interface{}, len(ref)) // their appropriate fields pointer
	for rows.Next() {
		for i := 0; i < l; i++ { // create new struct variable
			tmp[i] = reflect.New(typ[i])
			rsa[i] = reflect.Append(rsa[i], tmp[i])
		}
		for i := 0; i < len(ref); i++ {
			if ref[i].name == nil { // NULL column
				ptr[i] = null
			} else {
				p := unsafe.Pointer(tmp[ref[i].name.(int)].Pointer() +
					ref[i].offset)
				ptr[i] = reflect.NewAt(ref[i].typ, p).Interface()
			}
		}
		if err = rows.Scan(ptr...); err != nil {
			return fmt.Errorf("Scan: %v", err)
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("Scan: %v", err)
	}

	// write to dest
	for i := 0; i < l; i++ {
		reflect.ValueOf(dest[i]).Elem().Set(rsa[i])
	}
	return nil
}

// scanField 根据rows.Columns()和映射，返回ts中适合 Scan的字段参考信息。
// 如果在当前struct中未找到某列名的映射，则必须在其紧接着的struct中找到，
// 否则违背Scan约定。
func scanField(rows *sql.Rows, ts []reflect.Type) ([]entryT, error) {
	col, _ := rows.Columns()
	ref := make([]entryT, len(col))
	var i, j int // i for col, j for ts
	var v entryT
	var ok bool
	stru := ts[0].Name()
	for ; i < len(col) && j < len(ts); i++ {
		if col[i] == "" { // delemiter of tables, move to next struct
			j++
			if j == len(ts) {
				break
			}
			stru = ts[j].Name()
			continue
		}
		dot := strings.LastIndex(col[i], ".")
		if dot != -1 { // eliminate table name & trans to lowercase
			col[i] = strings.ToLower(col[i][dot+1:])
		} else {
			col[i] = strings.ToLower(col[i])
		}
		// mapping must exist in the current or the successive struct
		if v, ok = mapping["1."+stru+"."+col[i]]; !ok {
			j++
			if j == len(ts) {
				return nil, fmt.Errorf("column %q has no mapping", col[i])
			}
			stru = ts[j].Name()
			if v, ok = mapping["1."+stru+"."+col[i]]; !ok {
				return nil, fmt.Errorf("column %q has no mapping", col[i])
			}
		}
		ref[i].name = j
		ref[i].offset = v.offset
		ref[i].typ = v.typ
	}
	if i < len(col) {
		return nil, fmt.Errorf("column %v has no mapping", col[i:])
	}
	return ref, nil
}

// Buildstr 为单表SQL INSERT、UPDATE语句，将data的 field字段拼接成符合规范的
//（赋）值串。field缺省时拼接所有映射字段。返回值：
// data为切片时："(列名1,列名2,...) VALUES (值1,值2,...),..."，用于INSERT。
// data为单值时："SET 列名1=值1,列名2=值2,..."，用于INSERT、UPDATE。
//
// 约定：
//	● data的类型形如[]*struct或*struct。
//	● field为嵌套结构成员时要写全名，即前缀除最外层的所属结构名。
//
// 注意：Buildstr不限制结果字符串的长度，调用者需防止SQL语句超长。
func Buildstr(data interface{}, field ...string) (string, error) {
	v := reflect.ValueOf(data)
	t := v.Type()
	if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Ptr &&
		t.Elem().Elem().Kind() == reflect.Struct {
		if v.Len() == 0 {
			return "", fmt.Errorf("Buildstr: data is nil")
		}
		return valuebuild(v, field...)
	}
	if t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct {
		return setbuild(v, field...)
	}
	return "", fmt.Errorf("Buildstr: argument 'data' bad type %q", t)
}

// valuebuild equivalent to Buildstr, but just for []*struct.
func valuebuild(v reflect.Value, field ...string) (string, error) {
	stru := v.Type().Elem().Elem().Name() // record struct name
	if e, ok := mapping[stru]; ok {
		if len(field) == 0 { // default all mapped fields
			field = e.name.([]string)
		}
	} else {
		return "", fmt.Errorf("Buildstr: %q has no mapping", stru)
	}

	// build: "(col1,col2,...) VALUES ("
	var sql strings.Builder
	sql.WriteString("(")
	for i, n := range field {
		if i > 0 {
			sql.WriteString(",")
		}
		if m, ok := mapping["0."+stru+"."+n]; ok {
			sql.WriteString(m.name.(string))
		} else {
			return "", fmt.Errorf("Buildstr: %q has no field %q", stru, n)
		}
	}
	sql.WriteString(") VALUES (")

	// build others
	for i := 0; i < v.Len(); i++ {
		if v.Index(i).IsNil() {
			return "", fmt.Errorf("Buildstr: data[%d] is nil", i)
		}
		b := v.Index(i).Pointer() // base address
		if i > 0 {
			sql.WriteString("),(")
		}
		for j, n := range field {
			if j > 0 {
				sql.WriteString(",")
			}
			m := mapping["0."+stru+"."+n]
			ptr := reflect.NewAt(m.typ, unsafe.Pointer(b+m.offset))
			if err := buildstr(&sql, "", ptr); err != nil {
				return "", fmt.Errorf("Buildstr: %v", err)
			}
		}
	}

	return sql.String() + ")", nil
}

// setbuild equivalent to Buildstr, but just for *struct.
func setbuild(v reflect.Value, field ...string) (string, error) {
	stru := v.Type().Elem().Name() // record struct name
	if e, ok := mapping[stru]; ok {
		if len(field) == 0 { // default all mapped fields
			field = e.name.([]string)
		}
	} else {
		return "", fmt.Errorf("Buildstr: %q has no mapping", stru)
	}

	var sql strings.Builder
	sql.WriteString("SET ")
	b := v.Pointer() // base address
	for j, n := range field {
		if j > 0 {
			sql.WriteString(",")
		}
		m, ok := mapping["0."+stru+"."+n]
		if !ok {
			return "", fmt.Errorf("Buildstr: %q has no field %q", stru, n)
		}
		ptr := reflect.NewAt(m.typ, unsafe.Pointer(b+m.offset))
		if err := buildstr(&sql, m.name.(string)+"=", ptr); err != nil {
			return "", fmt.Errorf("Buildstr: %v", err)
		}
	}

	return sql.String(), nil
}

// buildstr 向b写入一条符合 SQL规范的（赋）值串。s为“列名=”或“”。
// v 可以是实现了driver.Valuer接口的类型值，及反射Kind为 Bool、Int、Uint、
// Float、String的“简单”类型或其指针，其它报错。
func buildstr(b *strings.Builder, s string, v reflect.Value) error {
	if f, ok := v.Interface().(driver.Valuer); ok {
		val, _ := f.Value()
		fmt.Fprintf(b, "%s%#v", s, val)
		return nil
	}

	v = reflect.Indirect(v)
	switch v.Kind() {
	case reflect.Bool:
		fmt.Fprintf(b, "%s%t", s, v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Int64:
		fmt.Fprintf(b, "%s%d", s, v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32,
		reflect.Uint64:
		fmt.Fprintf(b, "%s%d", s, v.Uint())
	case reflect.Float32, reflect.Float64:
		fmt.Fprintf(b, "%s%g", s, v.Float())
	case reflect.String:
		fmt.Fprintf(b, "%s%q", s, v.String())
	default:
		return fmt.Errorf("type %q cannot be valued", v.Type())
	}
	return nil
}
