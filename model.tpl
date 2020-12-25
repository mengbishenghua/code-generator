package {{.PackageName}}

{{ $isimport := isImport .Fields }}{{if $isimport }}import ("time"){{end}}
{{$tagfield := .TagFields}}

// {{.TableInfo.Name}} {{.TableInfo.Comment}}
type {{.TableInfo.Name}} struct {
{{range $index, $field := .Fields}}
    {{ $field.Name}} {{ $field.Type}} `json:"{{index $tagfield $index}}" gorm:"{{index $tagfield $index }}"` // {{.Desc}}{{end}}
}