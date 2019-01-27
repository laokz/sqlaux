package sqlaux

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"unicode"
)

// Scan 是对标准库*sql.Rows.Scan()等方法的再包装，以便于更简捷地接收查询结果。
// Scan 从已执行完查询的rows中，接收当前结果集（即一个单独的SELECT）的所有结果，
// 并追加到dest所指向的数据切片中。接收后Scan 不主动关闭rows，这样如果还有别的
// 结果集，可继续对rows进行操作。
//
// 约定与示例:
//	type T struct {...}
//	type T1 struct {...}				 // 对应数据库表t1
//	type T2 struct {..., T, ...}		 // 可以嵌套struct
//	var v1 []*T1; var v2 []*T2
//	rs, _ := db.Query(`SELECT t1.*,		 // 逐表罗列选择列*
//	         t2.C1,t2.C2 FROM t1,t2...`)
//	Scan(rs, &v1, &v2)	 				 // dest的类型形如*[]*struct
//										 // 接收参数一一对应SELECT后的表**
// *选择列时，如果两表“交界”处的列名如C1，在T1、T2中有相同的映射名，则Scan 可能将结
// 果赋予v1而不是v2，为避免这种情况可在表间用空列''分隔，如：SELECT t1.*,'',t2.C1
// **默认Scan对结构字段名和数据库列名进行大小写不敏感的相等匹配，使用StructMap可以
// 定制映射关系。
func Scan(rows *sql.Rows, dest ...interface{}) error {
	// 创建接收结果的临时变量
	if len(dest) == 0 {
		return fmt.Errorf("Scan: dest接收参数不能为空")
	}
	tmp := make([]reflect.Value, len(dest)) // 用于每次rows.Scan的变量
	rsa := make([]reflect.Value, len(dest)) // 用于累积结果的变量
	for i, r := range dest {
		t := reflect.TypeOf(r) // *[]*struct
		if t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Slice ||
			t.Elem().Elem().Kind() != reflect.Ptr ||
			t.Elem().Elem().Elem().Kind() != reflect.Struct {
			return fmt.Errorf("Scan: dest[%d]参数非结构指针切片地址", i)
		}
		tmp[i] = reflect.New(t.Elem().Elem().Elem())  // *struct
		rsa[i] = reflect.Indirect(reflect.ValueOf(r)) // []*struct
	}

	// 根据查询结果确定具体的接收字段地址。使用固定的变量可省去每次Scan都要确定地址的步骤
	ptr, err := prepareVars(rows, tmp...)
	if err != nil {
		return err
	}

	// 接收所有结果
	for rows.Next() {
		if err = rows.Scan(ptr...); err != nil {
			return err
		}
		for i, v := range tmp { // 将接收到的值拷贝并追加到累积结果变量中
			t := v.Elem().Type() // struct
			vv := reflect.New(t)
			vv.Elem().Set(v.Elem())
			rsa[i] = reflect.Append(rsa[i], vv)
			v.Elem().Set(reflect.Zero(t)) // 清零固定的接收变量
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}

	// 将累积的结果赋予接收参数
	for i := 0; i < len(dest); i++ {
		reflect.ValueOf(dest[i]).Elem().Set(rsa[i])
	}
	return nil
}

// prepareVars 根据rows.Columns()的列名，返回*struct e中适合 Scan的字段地址切片。
// 约定：列名逐表排列，表间可用空列''分隔。
func prepareVars(rows *sql.Rows, e ...reflect.Value) ([]interface{}, error) {
	cols, _ := rows.Columns()                 // 结果列
	var r = make([]interface{}, 0, len(cols)) // 接收字段地址切片
	var null = new(string)                    // 占位空字段

	var i, j int // i遍历结果列，j遍历接收结构
	for ; i < len(cols) && j < len(e); i++ {
		if cols[i] == "" { // 遇到表间隔标识后，下次将在下个struct中查找合适的字段指针
			r = append(r, null)
			j++
			continue
		}
		f := getFieldAddr(cols[i], e[j]) // 在当前struct查找
		if f == nil {                    // 未找到，在下个struct找
			if j == len(e)-1 {
				return nil, fmt.Errorf("列%s无匹配字段，请检查整个数据映射", cols[i])
			}
			if f = getFieldAddr(cols[i], e[j+1]); f == nil {
				return nil, fmt.Errorf("列%s无匹配字段，请检查整个数据映射", cols[i])
			}
			j++
		}
		r = append(r, f)
	}
	if i < len(cols) {
		return nil, fmt.Errorf("列%s无匹配字段，请检查整个数据映射", cols[i:])
	}
	return r, nil
}

// StructMap 提供GO struct到数据库表的名称映射，默认为nil。键值串的格式为“xxx.xxx”，
// 对于key表示的是“结构名.字段名”，对于value表示的是“数据库表名.列名”，其中结构名和数
// 据库表名均可以为空，表示不做限制，字段必须为导出字段，点.不能省略。系统忽略不合法的映
// 射而不报错。匹配规则：
//	1. 查找指定的“结构名.字段名”的映射
//	2. 查找不限制结构名的“.字段名”的映射
//	3. 对于有映射的，如为期望的“表名.列名”或“.列名”，则成功，否则失败
//  4. 对于没有映射的，对“字段名”和“列名”进行大小写不敏感的相等匹配
//
// 注意：如无特别配置，*sql.Rows.Columns()不包含表名，因此通常不应限制映射的表名。
// StructMap 能解决GO数据结构与数据库表列名称不一致的问题，但不能解决SELECT选择列歧义问
// 题，见Scan注释。
var StructMap map[string]string

// getFieldAddr 返回*struct e中与查询结果列名 n对应的导出字段地址，未找到时返回nil。
// 查找时遍历e的嵌套 struct，包括非匿名struct或* struct。如果StructMap有值，则先查找
// 这个映射，否则对“字段名”和“列名”进行大小写不敏感的相等匹配。
func getFieldAddr(n string, e reflect.Value) interface{} {
	v := reflect.Indirect(e)

	i := strings.Index(n, ".") // rows列名可能包含表名
	table := ""
	if i != -1 {
		table = n[:i]
	}
	col := n[i+1:]
	stru := v.Type().Name()
	f := v.FieldByNameFunc(func(field string) bool { // 在e及其匿名结构字段中找
		if !unicode.IsUpper([]rune(field)[0]) { // 跳过非导出字段。field总不为空
			return false
		}
		if StructMap == nil { // 无映射
			return strings.ToLower(field) == strings.ToLower(col)
		}
		if tc, ok := StructMap[stru+"."+field]; ok { // 有指定的映射
			tcs := strings.Split(tc, ".")
			if len(tcs) == 2 && tcs[1] == col && (tcs[0] == "" ||
				tcs[0] == table) { // 合法且列名匹配、表名相同或不限制
				return true
			}
		}
		if tc, ok := StructMap["."+field]; stru != "" && ok { // 有不限制映射
			tcs := strings.Split(tc, ".")
			if len(tcs) == 2 && tcs[1] == col && (tcs[0] == "" ||
				tcs[0] == table) {
				return true
			}
		}
		return strings.ToLower(field) == strings.ToLower(col)
	})
	if f.IsValid() {
		return f.Addr().Interface()
	}
	for i := 0; i < v.NumField(); i++ { // 在e的非匿名结构字段继续找
		vv := reflect.Indirect(v.Field(i))
		if vv.Kind() == reflect.Struct && !v.Type().Field(i).Anonymous {
			if r := getFieldAddr(n, vv); r != nil {
				return r
			}
		}
	}
	return nil
}
