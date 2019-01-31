# sqlaux
### Description
这段小程序仅仅提供了GOLANG数据库相关的两个函数和一个数据结构，目的是更加方便地接收各种查询结果、插入更新各类数据，辅助程序员构建自己的I(nsert)D(elete)U(pdate)S(elect)库。


GOLANG sql标准包提供的数据库操作简单而直接，但在进行查询操作时，由于必须准备与*sql.Rows结果列完全一致的变量，因此当面对经常使用而又需求多变的SELECT操作时，sql包就显得非常繁琐。INSERT、UPDATE罗列那些字段实在是令人头疼。

在试验其它如ORM、BUILDER等类型的扩展包时，有的比较复杂，学习跟踪成本比较高，有的非常面向对象而遮蔽了SQL本身直接的逻辑。

因此写了这个辅助函数，期望既能保持数据库操作的简单直接，又减少不必要的编程负担，可以说是用小的时空成本换取编程效率的提升。对于希望保持语句可控，逻辑简捷，代码轻量者是有益的。整个包实际不过200多行。

### Example
1. SELECT

假设系统有三个数据结构：

	type T struct {...}
	type T1 struct {...}
	type T2 struct {..., T, ...}

T1、T2分别对应着数据库表t1和t2

现在定义变量并进行查询：

	var v1 []*T1; var v2 []*T2
	rs, _ := db.Query(`SELECT t1.*,t2.C1,t2.C2 FROM t1,t2...`)

接收结果是简单而直接的：

	err := Scan(rs, &v1, &v2)	

现在就可以使用保存在v1、v2中的结果了。

2. INSERT

假设有自定义类型的字段要作为一个值插入数据库：

	type T struct {
		...
		f MyType
		...
	}
	type MyType {...}
	func (s MyType) String() string {...} // 实现fmt.Stringer接口

将包含该字段的值插入数据库：

	var v = T{..., f:..., ...}
	vstr, _ := Buildstr(true, &v)
	db.Exec(`INSERT INTO t VALUES ` + vstr) // f的值会按String()方法转换



默认Scan对结构字段名和数据库列名进行大小写不敏感的相等匹配，使用StructMap可以定制自己的映射关系。

在一个应用中，GO数据结构与数据库的对应关系通常是固定的，因此以上的函数运用可以：简化标准包中的繁琐操作、保持SQL语句的直观、提供灵活的应用定制功能。

### Appreciate
如果觉得有BUG，请告诉我，我将非常感谢你的帮助！如果使用了它，也请告诉我，我将感到非常荣幸:)。