package sqlaux

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type Customer struct {
	UID       uint32 `db:"pk"`             // 客户编号，自动生成
	Co        string `db:"nonull len=100"` // 公司名
	aa        int
	Trade     string    `db:"nonull len=20"`  // 行业
	Region    string    `db:"nonull len=20"`  // 地区
	Joined    time.Time ``                    // 加入时间
	Contactor string    `db:"nonull len=20"`  // 联系人
	Contact   string    `db:"nonull len=100"` // 联系方式
	Memo      string    `db:"len=300"`        // 备注
}

// Email 表示邮件及其发送信息。
type Email struct {
	MessageID string        `db:"pk len=200"`     // 自动生成，攻击链接中应嵌入该信息
	UID       uint32        ``                    // 客户编号
	OrderID   uint16        ``                    // 订单号
	Addr      string        `db:"nonull len=30"`  // 目标SMTP服务器地址
	To0       strslice      `db:"nonull len=100"` // 真收件人
	Time      time.Time     ``                    // 计划发送时间
	Repeats   uint8         ``                    // 重发次数，默认0-不重发
	Duration  time.Duration ``                    // 重发间隙
	Status    int8

	Material
}

// Material 表示邮件模板。除头两项，所有字段均为伪造。
// MID=1 的模板为默认模板。
type Material struct {
	MID uint16 `db:"pk"`             // 模板编号，自增长，不为0
	Tag string `db:"nonull len=100"` // 空格分隔的模板标签，用于自动匹配目标

	// 基本信息
	// 邮件中标记的发送时区，如：+0800、-0730，应与发件人身份吻合
	Zone int16 ``
	// 发件人，逗号分隔，格式：[名字]<地址>，程序固定用第1个 Froms作为发件人
	Froms   strslice `db:"nonull len=100"`
	To1     strslice `db:"len=400"` // 收件人，逗号分隔
	Subject string   `db:"nonull len=100"`
	// 正文，内嵌{{.name}}、{{.messageID}}字段，生成邮件时用实际值替换
	// 内嵌诱饵链接应包括需要上传的邮件标识、用户邮件地址、链接标记等参数
	// 形如：https://domain/link?mid={{.messageID}}&email=xxx&flag=xxx
	Body string `db:"nonull"`
	// 附件，逗号分隔的文件名列表。服务端实际存储的文件为：$uploadRoot/path/原名@显示名
	Attachment strslice `db:"len=100"`

	// 更多信息
	Sender   string   `db:"len=30"`  // 发件人，单个地址
	ReplyTo  strslice `db:"len=100"` // 回复地址Reply-To，逗号分隔
	Cc       strslice `db:"len=100"` // 抄送
	Comments string   `db:"len=200"` // 注释
	Keywords string   `db:"len=50"`  // 关键字，逗号分隔
	Received string   `db:"len=200"` // 传递路径信息
}

func TestSelect(t *testing.T) { //
	db, err := sql.Open("mysql", "test:test@/test?charset=utf8&columnsWithAlias=true&loc=Local&parseTime=true") //????????????
	if err != nil {
		t.Fatal(err)
	}

	StructMap = map[string]string{"Customer.Co": "Customer.Com"}

	var cs = []*Customer{&Customer{Co: "AAA"}}
	var or []*Email
	rows, err := db.Query(`SELECT Com,Email.Zone,Email.Froms,Email.To0,Email.Time FROM Customer,
			Email WHERE Customer.UID=Email.UID `)
	if err != nil {
		t.Fatal(err)
	}
	r, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%v\n", r)
	err = Scan(rows, &cs, &or)
	rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(cs); i++ {
		fmt.Printf("Com: %s\n", cs[i].Co)
	}
	for i := 0; i < len(or); i++ {
		fmt.Printf("Zone: %d\n", or[i].Zone)
		fmt.Printf("Froms: %v\n", or[i].Froms)
		fmt.Printf("To0: %v\n", or[i].To0)
		fmt.Printf("Time: %v\n\n", or[i].Time)
	}
}

// strslice 及其scan方法用于从数据库字段中接收并转换为切片字符串。
type strslice []string

func (s *strslice) Scan(v interface{}) error {
	if v == nil {
		return nil
	}
	var t []string
	switch str := v.(type) {
	case string:
		t = strings.Split(str, ",")
	case []byte:
		t = strings.Split(string(str), ",")
	default:
		return fmt.Errorf("Scan strslice：期望字符串类型，但得到的是%s",
			reflect.TypeOf(v).Kind())
	}
	if len(t) > 0 && t[0] != "" {
		*s = t
	}
	return nil
}
