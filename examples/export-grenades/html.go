package main

import (
	"fmt"
	"html/template"
	"io"
	"os"
)

type htmlRow struct {
	Map         string
	DemoPath    string
	GrenadeType string
	Thrower     string
	StartX      string
	StartY      string
	StartZ      string
	AngleX      string
	AngleY      string
	AngleZ      string
	DetonateX   string
	DetonateY   string
	DetonateZ   string
	Category    string
	CopyText    string
}

func writeHTMLFile(path string, records []GrenadeRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create HTML file: %w", err)
	}
	defer f.Close()

	return writeHTML(f, records)
}

func writeHTML(w io.Writer, records []GrenadeRecord) error {
	rows := make([]htmlRow, 0, len(records))
	for _, rec := range records {
		rows = append(rows, htmlRow{
			Map:         rec.Map,
			DemoPath:    rec.DemoPath,
			GrenadeType: rec.GrenadeType,
			Thrower:     rec.Thrower,
			StartX:      formatCoord(rec.Start.X),
			StartY:      formatCoord(rec.Start.Y),
			StartZ:      formatCoord(rec.Start.Z),
			AngleX:      formatCoord(rec.ViewAngles.X),
			AngleY:      formatCoord(rec.ViewAngles.Y),
			AngleZ:      formatCoord(rec.ViewAngles.Z),
			DetonateX:   formatCoord(rec.Detonate.X),
			DetonateY:   formatCoord(rec.Detonate.Y),
			DetonateZ:   formatCoord(rec.Detonate.Z),
			Category:    rec.Category,
			CopyText:    copyPayload(rec),
		})
	}

	err := grenadeHTML.Execute(w, rows)
	if err != nil {
		return fmt.Errorf("failed to write HTML: %w", err)
	}

	return nil
}

var grenadeHTML = template.Must(template.New("grenades").Parse(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>道具标注</title>
<style>
body { font-family: sans-serif; margin: 16px; }
table { border-collapse: collapse; width: 100%; font-size: 13px; }
th, td { border: 1px solid #ccc; padding: 4px 8px; white-space: nowrap; }
th { position: sticky; top: 0; background: #f4f4f4; }
button { cursor: pointer; }
button.copied { background: #c8f7c5; }
</style>
</head>
<body>
<p>共 {{len .}} 条道具。点击行尾「复制」可复制投掷者坐标和准星角度（setpos / setang）。</p>
<table>
<thead>
<tr>
<th>道具所属地图</th>
<th>道具所属demo</th>
<th>道具种类</th>
<th>道具投掷者</th>
<th>起点X</th><th>起点Y</th><th>起点Z</th>
<th>准星角度X</th><th>准星角度Y</th><th>准星角度Z</th>
<th>爆点X</th><th>爆点Y</th><th>爆点Z</th>
<th>道具分类</th>
<th></th>
</tr>
</thead>
<tbody>
{{range .}}
<tr>
<td>{{.Map}}</td>
<td>{{.DemoPath}}</td>
<td>{{.GrenadeType}}</td>
<td>{{.Thrower}}</td>
<td>{{.StartX}}</td><td>{{.StartY}}</td><td>{{.StartZ}}</td>
<td>{{.AngleX}}</td><td>{{.AngleY}}</td><td>{{.AngleZ}}</td>
<td>{{.DetonateX}}</td><td>{{.DetonateY}}</td><td>{{.DetonateZ}}</td>
<td>{{.Category}}</td>
<td><button type="button" data-copy="{{.CopyText}}" onclick="copyRow(this)">复制</button></td>
</tr>
{{end}}
</tbody>
</table>
<script>
function copyRow(btn) {
  var text = btn.getAttribute("data-copy") || "";
  var done = function() {
    btn.textContent = "已复制";
    btn.classList.add("copied");
    setTimeout(function() {
      btn.textContent = "复制";
      btn.classList.remove("copied");
    }, 1200);
  };
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(done).catch(function() { fallbackCopy(text, done); });
    return;
  }
  fallbackCopy(text, done);
}
function fallbackCopy(text, done) {
  var ta = document.createElement("textarea");
  ta.value = text;
  document.body.appendChild(ta);
  ta.select();
  try { document.execCommand("copy"); } catch (e) {}
  document.body.removeChild(ta);
  done();
}
</script>
</body>
</html>
`))
