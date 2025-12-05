package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"time"
)

// GenerateJSON generates a JSON report.
func GenerateJSON(data *ReportData) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

// GenerateHTML generates an HTML report.
func GenerateHTML(data *ReportData) ([]byte, error) {
	tmpl, err := template.New("report").Funcs(template.FuncMap{
		"formatDate": func(t time.Time) string {
			return t.Format("January 2, 2006 15:04 MST")
		},
		"formatNumber": func(n int64) string {
			return formatNumberWithCommas(n)
		},
		"formatPercent": func(f float64) string {
			return fmt.Sprintf("%.1f%%", f)
		},
		"formatFloat": func(f float64) string {
			return fmt.Sprintf("%.2f", f)
		},
		"truncate": truncate,
	}).Parse(htmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	return buf.Bytes(), nil
}

func formatNumberWithCommas(n int64) string {
	if n < 0 {
		return "-" + formatNumberWithCommas(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	var parts []string
	for n > 0 {
		parts = append([]string{fmt.Sprintf("%03d", n%1000)}, parts...)
		n /= 1000
	}

	// Remove leading zeros from first part
	if len(parts) > 0 {
		for len(parts[0]) > 1 && parts[0][0] == '0' {
			parts[0] = parts[0][1:]
		}
	}

	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ","
		}
		result += p
	}
	return result
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Caddystat {{.ReportType}} Report</title>
    <style>
        :root {
            --bg-primary: #ffffff;
            --bg-secondary: #f3f4f6;
            --text-primary: #111827;
            --text-secondary: #6b7280;
            --border-color: #e5e7eb;
            --accent-color: #3b82f6;
            --success-color: #10b981;
            --warning-color: #f59e0b;
            --danger-color: #ef4444;
        }
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
            line-height: 1.6;
            color: var(--text-primary);
            background-color: var(--bg-primary);
            padding: 2rem;
            max-width: 1200px;
            margin: 0 auto;
        }
        header {
            margin-bottom: 2rem;
            padding-bottom: 1rem;
            border-bottom: 2px solid var(--border-color);
        }
        h1 {
            font-size: 1.875rem;
            font-weight: 700;
            margin-bottom: 0.5rem;
        }
        .subtitle {
            color: var(--text-secondary);
            font-size: 0.875rem;
        }
        .grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }
        .card {
            background: var(--bg-secondary);
            border-radius: 0.5rem;
            padding: 1rem;
        }
        .card-title {
            font-size: 0.75rem;
            color: var(--text-secondary);
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 0.25rem;
        }
        .card-value {
            font-size: 1.5rem;
            font-weight: 600;
        }
        .section {
            margin-bottom: 2rem;
        }
        h2 {
            font-size: 1.25rem;
            font-weight: 600;
            margin-bottom: 1rem;
            padding-bottom: 0.5rem;
            border-bottom: 1px solid var(--border-color);
        }
        table {
            width: 100%;
            border-collapse: collapse;
            font-size: 0.875rem;
        }
        th, td {
            text-align: left;
            padding: 0.75rem;
            border-bottom: 1px solid var(--border-color);
        }
        th {
            background: var(--bg-secondary);
            font-weight: 600;
        }
        tr:hover {
            background: var(--bg-secondary);
        }
        .number {
            text-align: right;
            font-variant-numeric: tabular-nums;
        }
        .status-success { color: var(--success-color); }
        .status-warning { color: var(--warning-color); }
        .status-danger { color: var(--danger-color); }
        .progress-bar {
            height: 8px;
            background: var(--bg-secondary);
            border-radius: 4px;
            overflow: hidden;
        }
        .progress-fill {
            height: 100%;
            background: var(--accent-color);
        }
        footer {
            margin-top: 3rem;
            padding-top: 1rem;
            border-top: 1px solid var(--border-color);
            text-align: center;
            color: var(--text-secondary);
            font-size: 0.75rem;
        }
        @media print {
            body { padding: 1rem; }
            .card { break-inside: avoid; }
            table { font-size: 0.75rem; }
        }
    </style>
</head>
<body>
    <header>
        <h1>Caddystat {{.ReportType | printf "%s" | title}} Report{{if .Host}} - {{.Host}}{{end}}</h1>
        <div class="subtitle">
            Period: {{formatDate .PeriodStart}} to {{formatDate .PeriodEnd}}
        </div>
    </header>

    <section class="section">
        <div class="grid">
            <div class="card">
                <div class="card-title">Total Requests</div>
                <div class="card-value">{{formatNumber .Summary.TotalRequests}}</div>
            </div>
            <div class="card">
                <div class="card-title">Unique Visitors</div>
                <div class="card-value">{{formatNumber .Summary.UniqueVisitors}}</div>
            </div>
            <div class="card">
                <div class="card-title">Page Views</div>
                <div class="card-value">{{formatNumber .Summary.PageViews}}</div>
            </div>
            <div class="card">
                <div class="card-title">Bandwidth</div>
                <div class="card-value">{{.Summary.BandwidthHuman}}</div>
            </div>
            <div class="card">
                <div class="card-title">Avg Response Time</div>
                <div class="card-value">{{formatFloat .Summary.AvgResponseTime}} ms</div>
            </div>
            {{if gt .Summary.BounceRate 0}}
            <div class="card">
                <div class="card-title">Bounce Rate</div>
                <div class="card-value">{{formatPercent .Summary.BounceRate}}</div>
            </div>
            {{end}}
        </div>
    </section>

    <section class="section">
        <h2>Response Status Codes</h2>
        <div class="grid">
            <div class="card">
                <div class="card-title">2xx (Success)</div>
                <div class="card-value status-success">{{formatNumber .Summary.Status2xx}}</div>
            </div>
            <div class="card">
                <div class="card-title">3xx (Redirect)</div>
                <div class="card-value">{{formatNumber .Summary.Status3xx}}</div>
            </div>
            <div class="card">
                <div class="card-title">4xx (Client Error)</div>
                <div class="card-value status-warning">{{formatNumber .Summary.Status4xx}}</div>
            </div>
            <div class="card">
                <div class="card-title">5xx (Server Error)</div>
                <div class="card-value status-danger">{{formatNumber .Summary.Status5xx}}</div>
            </div>
        </div>
    </section>

    {{if .TopPages}}
    <section class="section">
        <h2>Top Pages</h2>
        <table>
            <thead>
                <tr>
                    <th>#</th>
                    <th>Path</th>
                    <th class="number">Requests</th>
                </tr>
            </thead>
            <tbody>
                {{range $i, $p := .TopPages}}
                {{if lt $i 20}}
                <tr>
                    <td>{{printf "%d" (add $i 1)}}</td>
                    <td>{{truncate $p.Path 80}}</td>
                    <td class="number">{{formatNumber $p.Count}}</td>
                </tr>
                {{end}}
                {{end}}
            </tbody>
        </table>
    </section>
    {{end}}

    {{if .TopReferrers}}
    <section class="section">
        <h2>Top Referrers</h2>
        <table>
            <thead>
                <tr>
                    <th>#</th>
                    <th>Referrer</th>
                    <th class="number">Hits</th>
                </tr>
            </thead>
            <tbody>
                {{range $i, $r := .TopReferrers}}
                {{if lt $i 15}}
                <tr>
                    <td>{{printf "%d" (add $i 1)}}</td>
                    <td>{{truncate $r.Referrer 60}}</td>
                    <td class="number">{{formatNumber $r.Hits}}</td>
                </tr>
                {{end}}
                {{end}}
            </tbody>
        </table>
    </section>
    {{end}}

    {{if .Browsers}}
    <section class="section">
        <h2>Browsers</h2>
        <table>
            <thead>
                <tr>
                    <th>Browser</th>
                    <th class="number">Hits</th>
                    <th class="number">Percent</th>
                </tr>
            </thead>
            <tbody>
                {{range .Browsers}}
                <tr>
                    <td>{{.Browser}}</td>
                    <td class="number">{{formatNumber .Hits}}</td>
                    <td class="number">{{formatPercent .Percent}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </section>
    {{end}}

    {{if .OperatingSystems}}
    <section class="section">
        <h2>Operating Systems</h2>
        <table>
            <thead>
                <tr>
                    <th>OS</th>
                    <th class="number">Hits</th>
                    <th class="number">Percent</th>
                </tr>
            </thead>
            <tbody>
                {{range .OperatingSystems}}
                <tr>
                    <td>{{.OS}}</td>
                    <td class="number">{{formatNumber .Hits}}</td>
                    <td class="number">{{formatPercent .Percent}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </section>
    {{end}}

    {{if gt .Bots.TotalHits 0}}
    <section class="section">
        <h2>Bot Traffic</h2>
        <div class="grid">
            <div class="card">
                <div class="card-title">Bot Hits</div>
                <div class="card-value">{{formatNumber .Bots.TotalHits}}</div>
            </div>
        </div>
    </section>
    {{end}}

    {{if .ErrorPages}}
    <section class="section">
        <h2>Error Pages</h2>
        <table>
            <thead>
                <tr>
                    <th>#</th>
                    <th>Path</th>
                    <th class="number">Status</th>
                    <th class="number">Count</th>
                </tr>
            </thead>
            <tbody>
                {{range $i, $e := .ErrorPages}}
                {{if lt $i 15}}
                <tr>
                    <td>{{printf "%d" (add $i 1)}}</td>
                    <td>{{truncate $e.Path 50}}</td>
                    <td class="number">{{$e.Status}}</td>
                    <td class="number">{{formatNumber $e.Count}}</td>
                </tr>
                {{end}}
                {{end}}
            </tbody>
        </table>
    </section>
    {{end}}

    {{if .Performance}}
    <section class="section">
        <h2>Performance Statistics</h2>
        <div class="grid">
            <div class="card">
                <div class="card-title">Min Response</div>
                <div class="card-value">{{formatFloat .Performance.ResponseTime.Min}} ms</div>
            </div>
            <div class="card">
                <div class="card-title">Avg Response</div>
                <div class="card-value">{{formatFloat .Performance.ResponseTime.Avg}} ms</div>
            </div>
            <div class="card">
                <div class="card-title">P95 Response</div>
                <div class="card-value">{{formatFloat .Performance.ResponseTime.P95}} ms</div>
            </div>
            <div class="card">
                <div class="card-title">Max Response</div>
                <div class="card-value">{{formatFloat .Performance.ResponseTime.Max}} ms</div>
            </div>
        </div>
    </section>
    {{end}}

    <footer>
        <p>Generated by Caddystat</p>
    </footer>
</body>
</html>`

func init() {
	// Register template functions
	template.Must(template.New("").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"title": func(s string) string {
			if len(s) == 0 {
				return s
			}
			if s[0] >= 'a' && s[0] <= 'z' {
				return string(s[0]-32) + s[1:]
			}
			return s
		},
	}).Parse(""))
}
