package main

import (
	"bytes"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io/ioutil"
	os "os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

var (
	db     *sql.DB
	err    error
	config *Config
)

const (
	queryTableInfoSql = `SELECT table_name name,IFNULL(TABLE_COMMENT,table_name) comment
 				FROM INFORMATION_SCHEMA.TABLES
				WHERE UPPER(table_type)='BASE TABLE'
				AND LOWER(table_schema) = ?
				ORDER BY table_name`
	columnsSql = `SELECT COLUMN_NAME fname,
       		column_comment fdesc,
       		DATA_TYPE ftype,
			IS_NULLABLE isNull,
       		COLUMN_TYPE fcolumntype,
       		IFNULL(CHARACTER_MAXIMUM_LENGTH,0) flength 
			FROM information_schema.columns 
			WHERE table_schema = ? AND table_name = ?
			ORDER BY ORDINAL_POSITION`
)

// Config 配置文件
type Config struct {
	WorkDir       string // 当前工作目录
	Path          string // 包路径,默认workdir
	PackageName   string // 包名/模块名
	AbsPath       string // 绝对路径: Path +  PackageName
	TableSchema   string // 要生成的表所在的数据库名
	DNS           string // 数据库连接
	DriverName    string // 数据库驱动，默认mysql,目前只支持mysql
	ModelTemplate string // model对象的模板名
	UpdateTime    string // 自定义gorm的updated_at时间追踪
	CreateTime    string // 自定义gorm的create_at时间追踪
}

// TableInfo 表信息
type TableInfo struct {
	Name    string // 表名
	Comment string //表描述
}

func (t *TableInfo) String() string {
	return fmt.Sprintf("name: %-16v, comment: %v\n", t.Name, t.Comment)
}

func (f *Field) String() string {
	return fmt.Sprintf("|%15v|%15v|%15v|%15v|%10v|%10v|", f.Name, f.Desc, f.Type, f.IsNull, f.ColumnType, f.Length)
}

// Field 表字段信息
type Field struct {
	Name       string // 字段名称
	Desc       string // 字段描述
	Type       string // 字段类型
	IsNull     string // 字段是否可空
	ColumnType string // 列的类型
	Length     int    // 字段长度
}

func init() {
	dir, _ := os.Getwd()
	config = &Config{
		WorkDir:       dir,
		Path:          dir,
		PackageName:   "model",
		DriverName:    "mysql",
		ModelTemplate: "model.tpl",
		CreateTime:    "CreateTime",
		UpdateTime:    "UpdateTime",
	}
	config.AbsPath = filepath.Clean(filepath.ToSlash(filepath.Join(config.Path, config.PackageName)))
}

// 生成路径,默认workdir
func SetPath(path string) {
	config.Path = path
	config.AbsPath = filepath.Join(config.Path, config.PackageName)
}

func SetPackageName(packageName string) {
	config.PackageName = packageName
	config.AbsPath = filepath.Join(config.Path, config.PackageName)
}

func SetDatabase(database string) {
	config.TableSchema = database
}

func SetDNS(dns string) {
	config.DNS = dns
}

// SetDriverName 目前只支持mysql
func SetDriverName(driverName string) {
	config.DriverName = driverName
}

// 连接数据库
func ConnectionDatabase() {
	db, err = sql.Open(config.DriverName, config.DNS)
	if err != nil {
		panic(err)
	}
	if err = db.Ping(); err != nil {
		panic(err)
	}
}

// 获取表信息
func getTableInfo(dbName string) []*TableInfo {
	rows, err := db.Query(queryTableInfoSql, dbName)
	if err != nil {
		panic(err)
	}
	var results []*TableInfo
	for rows.Next() {
		tableInfo := new(TableInfo)
		err := rows.Scan(&tableInfo.Name, &tableInfo.Comment)
		if err != nil {
			panic(err)
		}
		results = append(results, tableInfo)
	}
	return results
}

// 获取字段信息
func getColumnsInfo(tableSchema string, tableName string) []*Field {
	rows, err := db.Query(columnsSql, tableSchema, tableName)
	if err != nil {
		panic(err)
	}
	var fields []*Field
	for rows.Next() {
		field := new(Field)
		err = rows.Scan(&field.Name, &field.Desc, &field.Type, &field.IsNull, &field.ColumnType, &field.Length)
		if err != nil {
			panic(err)
		}
		fields = append(fields, field)
	}
	return fields
}

func Execute() {
	// 在这之前可以加上gui程序
	generate()
}

var funcs = template.FuncMap{
	"isImport": isImportTime,
	"autoTime": autoTime,
}

// 生成文件
func generate() {
	tmp, err := template.New("model.tpl").
		Funcs(funcs).
		ParseFiles(filepath.Join("model.tpl"))

	if err != nil {
		panic(err)
	}

	var tableOutInfos []*TableOutInfo

	tableInfos := getTableInfo(config.TableSchema)
	for _, tableInfo := range tableInfos {
		fields := getColumnsInfo(config.TableSchema, tableInfo.Name)

		tagFields := generateTagFields(fields)

		toi := &TableOutInfo{
			OriginTableName: tableInfo.Name,
			PackageName:     config.PackageName,
			TableInfo:       tableInfo,
			TagFields:       tagFields,
			Fields:          fields,
		}
		tableInfo.Name = convert(tableInfo.Name)
		tableOutInfos = append(tableOutInfos, toi)
	}

	if _, err := os.Stat(config.AbsPath); err == nil {
		fmt.Println("\033[31m清空目录文件 =>", config.AbsPath+"\033[0m")
		err := os.RemoveAll(config.AbsPath)
		if err != nil {
			fmt.Println(err)
		}
	}
	fmt.Println(config.AbsPath)
	err = os.MkdirAll(config.Path, os.ModePerm)
	// 临时保存文件，用于go fmt
	dir, err := ioutil.TempDir(config.WorkDir, config.PackageName)
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	var wg sync.WaitGroup
	for _, tableOutInfo := range tableOutInfos {
		wg.Add(1)
		go func(tableOutInfo *TableOutInfo) {
			defer wg.Done()
			file, err := os.Create(filepath.Join(dir, tableOutInfo.OriginTableName) + ".go")
			if err != nil {
				panic(err)
			}
			defer file.Close()

			err = tmp.Execute(file, tableOutInfo)
			if err != nil {
				panic(err)
			}
		}(tableOutInfo)
	}

	wg.Wait()
	fmt.Println("\033[32m生成成功 =>", config.AbsPath+"\033[0m")

	gofmt(dir)

	oldPath := dir
	newPath := filepath.Join(config.Path, config.PackageName)
	if err = moveDir(oldPath, newPath); err != nil {
		panic(err)
	}
	fmt.Println("\033[32mSuccess!\033[0m")
}

// 生成标签的映射名: `json:"createTime"`
func generateTagFields(fields []*Field) []string {
	var tagFields []string
	for _, field := range fields {
		tagField := tagFieldConvert(field.Name)
		tagFields = append(tagFields, tagField)

		field.Name = convert(field.Name)
		field.Type = typeConvert(field.Type)
	}
	return tagFields
}

// 将文件移动到指定目录: Path + PackageName
func moveDir(oldPath, newPath string) error {
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		os.MkdirAll(newPath, os.ModePerm)
	}

	fileList, err := getFileAndDirList(oldPath)
	if err != nil {
		panic(err)
	}
	for _, file := range fileList {
		filename := filepath.Base(file)
		data, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}
		if err = ioutil.WriteFile(filepath.Join(newPath, filename), data, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

func getFileAndDirList(path string) ([]string, error) {
	var res []string
	fileInfos, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() {
			list, err := getFileAndDirList(filepath.Join(path, fileInfo.Name()))
			if err != nil {
				return nil, err
			}
			res = append(res, list...)
		} else {
			res = append(res, filepath.Join(path, fileInfo.Name()))
		}
	}

	return res, nil
}

// 格式化文件
func gofmt(dir string) {
	cmd := exec.Command("go", "version")
	err = cmd.Run()
	if err != nil {
		fmt.Println("没有安装golang，无法进行格式化")
		return
	}
	cmd = exec.Command("go", "fmt", dir)
	cmd.Stdout = os.Stdout
	fmt.Println("执行go fmt命令...")
	err = cmd.Run()
	if err != nil {
		fmt.Printf("\033[31m运行go fmt失败: %v, 在目录: %v, 可能未在go.mod下运行go fmt命令\n\033[0m", err, config.AbsPath)
		return
	}
	fmt.Println("\033[32m执行go fmt成功!\033[0m")
}

// 转换_为驼峰命名, id字段比较特殊，字节转为全大写
func convert(input string) string {
	if input == "id" || input == "iD" || input == "Id" {
		return strings.ToUpper(input)
	}
	var buff bytes.Buffer
	splits := strings.Split(input, "_")
	for _, sp := range splits {
		sp = strings.Title(sp)
		buff.WriteString(sp)
	}
	return buff.String()
}

// 标签的映射名e.g.: `json:"createTime" gorm:"createTime"`
func tagFieldConvert(field string) string {
	var buff bytes.Buffer
	splits := strings.Split(field, "_")
	buff.WriteString(strings.ToLower(splits[0]))
	for i := 1; i < len(splits); i++ {
		buff.WriteString(strings.Title(splits[i]))
	}
	return buff.String()
}

// 数据库字段类型转go的结构体字段类型
func typeConvert(dataType string) string {
	switch dataType {
	case "varchar", "longtext", "char", "text":
		return "string"
	case "datetime", "date", "time":
		return "time.Time"
	case "tinyint":
		return "bool"
	case "int", "timestamp", "integer":
		return "int"
	case "bigint":
		return "int64"
	case "blob", "varbinary":
		return "[]byte"
	case "float":
		return "float32"
	case "double":
		return "float64"
	default:
		panic(dataType + " not convert")
	}
}

// 表输出信息
type TableOutInfo struct {
	OriginTableName string     // 原数据库字段名
	PackageName     string     // 包名， 默认model
	TableInfo       *TableInfo // 表信息
	Fields          []*Field   // 字段信息
	TagFields       []string   // 要生成标签的字段的映射名
}

// 如果有time.Time类型的话,导入time包，
func isImportTime(fields []*Field) bool {
	for _, field := range fields {
		if field.Type == "time.Time" {
			return true
		}
	}
	return false
}

// 自动追踪gorm autoCreateUpdate、autoUpdateTime
func autoTime(field string) string {
	switch field {
	case "UpdateTime":
		return "autoUpdateTime"
	case "CreateTime":
		return "autoCreateTime"
	default:

	}
	return ""
}
