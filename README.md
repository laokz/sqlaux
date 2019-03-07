# sqlaux
[![Build Status](https://travis-ci.com/LaoK996/sqlaux.svg?branch=master)](https://travis-ci.com/LaoK996/sqlaux)[![Coverage Status](https://coveralls.io/repos/github/LaoK996/sqlaux/badge.svg?branch=master)](https://coveralls.io/github/LaoK996/sqlaux?branch=master)
### Description
这段小程序仅仅提供了GOLANG数据库相关的几个函数和数据结构，目的是更加方便地接收各种查询结果、插入更新各类数据，辅助程序员构建自己的InsertDeleteUpdateSelect库。

特点：快、简捷

GOLANG sql标准包提供的数据库操作简单而直接，但在进行查询操作时，由于必须准备与*sql.Rows结果列完全一致的变量，因此当面对经常使用而又需求多变的SELECT操作时，sql包就显得非常繁琐。INSERT、UPDATE罗列那些字段实在是令人头疼。同时自定义类型的频繁类型转换也是一个麻烦事。

在试验其它如ORM、BUILDER等类型的扩展包时，有的比较复杂，学习跟踪成本比较高，有的非常面向对象而遮蔽了SQL本身直接的逻辑，有的则太重了。

因此写了这个辅助函数包，期望既能保持数据库操作的简单直接，又减少不必要的编程负担。在重新设计的过程中，特别考虑了时间性能问题，最终选择用预先初始化映射来减少实际数据库读写时的查找与反射，可以说是用小的空间成本换取编程效果的提升。对于希望保持语句可控，逻辑简捷，代码轻量者是有益的。整个包实际不过300多行。

### Example
假设系统有两个数据结构T1、T2分别对应着数据库表t1和t2。

1. 建立并初始化映射

第一步，通过tag标签建立名称映射：

	type T1 struct {
		F1 int    `db:"col=userid"`
		F2 string
		F3 string `db:"col=name"`
		...
	}
	type T2 struct {...}

默认Scan对结构字段名和数据库列名进行大小写不敏感的相等匹配，使用字段tag可以定制自己的映射关系，tag标记本身也可定制。

第二步，在init()函数中初始化映射：

	func init() {
		MapStruct(T1{}, T2{})	// 这里用了反射包，因此参数为变量值，可取零值
	}

2. SELECT

第一步，进行查询：

	rs, _ := db.Query(`SELECT t1.*,t2.C1,t2.C2 FROM t1,t2...`)

第二步，定义变量并接收结果：

	var v1 []*T1
	var v2 []*T2
	Scan(rs, &v1, &v2)	// Scan为接收变量自动分配内存

现在就可以使用保存在v1、v2中的结果了。

3. INSERT

第一步，构建值串：

	var vs = []*T1{...}		// []*struct的形式有利于切片底层数组动态分配，sqlaux均采用这一约定
	vstr, _ := Buildstr(vs)	// (列名1,列名2,...) VALUES (值1,值2,...),...   这里可选择插入哪些字段

第二步，执行插入语句：

	db.Exec(`INSERT INTO t1 ` + vstr)

在一个应用中，GO数据结构与数据库的对应关系通常是固定的，因此以上的函数运用可以：简化标准包中的繁琐操作、保持SQL语句的直观、提供灵活的应用定制功能。

### Tricks
GO sql包使用 sql.Scanner/driver.Valuer接口操作数据库读写，这样就需要程序自定义类型来实现这个接口，上面的例子中就可以直接使用这些自定义类型，读写数据库是简单了，但在程序其它逻辑中却经常需要类型转换，麻烦。

sqlaux 提供了一个映射函数MapType，可让程序两者兼得。比如，程序想直接使用字符串切片：

第一步，自定义类型，并实现sql.Scanner/driver.Valuer接口

	type T{..., Aslice []string, ...}		// Aslice使用 Go原生类型
	type mySlice []string					// 定义等价类型
	func (p *mySlice) Scan(...) {...}
	func (s mySlice) Value() ... {...}

第二步，在init()函数中建立这个类型映射：

	func init() {
		MapType([]string(nil), mySlice(nil))	// 这里也用到了反射包
	}

也可以对Go包提供的类型建立映射，比如time.Time在有些环境下支持的不好，可以自定义等价类型，实现相关接口，初始化映射，OK。

	MapType(time.Time{}, myTime{})

### Appreciate
如果有BUG，请告诉我，我将非常感谢！如果使用了它，也请告诉我，我将感到非常荣幸:)。

------------

### Description
This little program provides just a few functions and data structures related to the GOLANG database, which makes it easier to receive query results, insert and update data, and help programmers build their own I(nsert)D(elete)U(pdate)S(elect) packages.

Features: fast and simple

The operation provided by the GOLANG SQL package is simple and straightforward. But when it comes to query operations, the SQL package is cumbersome with the various SELECT demands, cause you have to prepare exactly the same variables as the result columns of *sql.rows. INSERT, UPDATE listing those fields is a real pain in the neck. Frequent type conversions of custom types are also a nuisance.

When experimenting with other types of extension packages such as ORM and BUILDER, some of them are more complex and have high learning and tracking costs, some are very object-oriented and obscure the direct logic of SQL itself, and some are too heavy.

Therefore, this package of auxiliary functions is written in the hope of keeping database operation simple and direct and reducing programming burden. Time performance is especially concerned. It use the pre-initialized mapping to reduce the overhead of search and reflection in DB I/O. Yes, it exchanges the improvement of programming effect with the small space cost. For those who want to keep their code controllable, simple, and light, this is beneficial. The entire package is actually just over 300 lines long.

### Example
Suppose there has two data structures T1 and T2 corresponding to database tables T1 and T2 respectively.

1. Establish and initialize the NAME mapping

A. Name mapping is established through tag:

	type T1 struct {
		F1 int    `db:"col=userid"`
		F2 string
		F3 string `db:"col=name"`
		...
	}
	type T2 struct {...}

Scan() performs case-insensitive matching between structure field and database column names by default. Use field tag can customize your own mapping. Tag itself can also be customized.

B. Initialize the mapping in the init() function:

	func init() {
		MapStruct(T1{}, T2{})	// using reflect
	}

2. SELECT

A. Query:

	rs, _ := db.Query(`SELECT t1.*,t2.C1,t2.C2 FROM t1,t2...`)

B. Define variables and receive results:

	var v1 []*T1
	var v2 []*T2
	Scan(rs, &v1, &v2)	// Scan allocate memory automatically

Now, you can use the results in v1 v2.

3. INSERT

A. Build strings:

	var vs = []*T1{...}		// []*struct is feasible for memory
	vstr, _ := Buildstr(vs)	// (col1,col2,...) VALUES (val1,val2,...),...   it can also choose which fields

B. Do the execution:

	db.Exec(`INSERT INTO t1 ` + vstr)

In an application, the relationship between the GO data structure and the database is usually fixed, so the above functions can be used anytime.

### Tricks
With non-standard type, GO SQL packages using sql.Scanner/driver.Valuer interface for DB I/O. So you need to custom type to implement them. Saying DB I/O is easy, but in other places you may often need to do type conversion.

Sqlaux provides a mapping function, MapType, that lets your program do both. For example, the program wants to use []string directly:

A. Define type, implement the two interfaces

	type T{..., Aslice []string, ...}		// GO primitive type
	type mySlice []string					// equivalent type
	func (p *mySlice) Scan(...) {...}
	func (s mySlice) Value() ... {...}

B. Initialize in init()

	func init() {
		MapType([]string(nil), mySlice(nil))	// also by reflection
	}

It is also possible to establish a mapping for the types provided by the Go standard package. For example, suppose time.Time is not well supported in some environments.

	MapType(time.Time{}, myTime{})

### Appreciate
If there is a BUG, tell me, I'll be very grateful! If it helped, tell me too, it's my pleasure :).

翻译支持：有道翻译(http://fanyi.youdao.com/)
