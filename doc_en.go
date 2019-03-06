/*
Package SQLAUX implements several fast and lightweight SQL helper functions
to more easily receive query results from the database, insert update data
into the database, and assist programmers to build their own InsertDelete
UpdateSelect package. SQLAUX guarantees the time performance of the
function by minimizing reflection package calls at a small space cost.

SQLAUX assumes that the data structure of the application has a one-to-one
correspondence with the database tables; However, it allows one data
structure to correspond to multiple tables with different isomorphic names
(table columns have exactly the same definition, table names are different),
such as history, etc.

To avoid letter case ambiguity, SQLAUX also converts all column names to
lowercase for reprocessing. Mapping also does not include table names,
because query results often do not contain table name information, and
programmers need to ensure that the correct struct to the correct table.

SQLAUX supports two kinds of mappings:

1. name map: lowercase(column name)!=lowercase(field name). It is required.
SQLAUX identifies the corresponding relationship between fields and column
names through struct tag. By default, all exported field names (including
nested structure members except time. time) are lowercased as their
corresponding DB table column names.

2. type map: field type cannot be directly used for DB I/O. It is optional.
In general, fields should use custom types at this point, and implement the
sql.scanner/driver.valuer interfaces, which do not need to be mapped. But
for Go native types such as slices, using custom types directly can cause a
lot of type conversion. These two interfaces are only used for DB I/O, so
SQLAUX provides a type-mapping method that allows the program to continue
using the Go native type, while using its equivalent custom type for DB I/O
within SQLAUX.

var (
	Tag = "db"
	Key = "col"
	Op  = "="
)

The three export variables are for struct tag, used to identify the DB
column names. Tag is tag name; Key is key of column name; Op is key-value
separator. eg, `db:"col=xxx yyy=zzz"`

func MapStruct(stru ...interface{}) error

MapStruct establishes name mappings between Go struct and DB table. The
caller needs to run it for each struct in init() for explicit mapping.
stru is struct needs to be mapped and takes the form of a variable value,
which can be zero value.

func MapType(orig, self interface{}) error

MapType establishes type mappings for Go struct and DB table. The caller
needs to call this in init() for each case, where the Go primitive type is
used in the program and the custom type is used for DB I/O.
orig, self are primitive and custom type values, respectively, and you can
use zero values.
sql.Scanner receiver is pointer type, driver.Valuer receiver is value type.
SQLAUX convention uses T instead of *T here.

func Scan(rows *sql.Rows, dest ...interface{}) error

Scan receives all the results of the current result set (that is, a single
SELECT) from rows where the query has been executed, overwriting dest with
the results. Scan does not actively close rows after receiving.
Convention:
	● Each dest type takes the form of *[]*struct.
	● Table by table when SELECT columns, the tables are optionally
	separated by '' empty columns. When two tables have duplicate names at
	the "junction", SQLAUX treats them as columns of the previous table by
	default. Separating them with '' can avoid duplicate names.

func Buildstr(data interface{}, field ...string) (string, error)

Buildstr for single-table SQL INSERT, UPDATE statement, concatenate the
data's field into right SQL string, the result takes the form:
 "(col1, col2,...) VALUES (val1, val2... ),... "
By default, all mapped fields will be dumped.
Convention:
	● The data type is like []*struct.
	● When a field is a member of nested struct, write its full name, that
	is, prefix its parent struct names except the outermost.
Note: Buildstr does not limit the length of the result string, and callers
need to prevent SQL statements from getting too long.
*/
package sqlaux
