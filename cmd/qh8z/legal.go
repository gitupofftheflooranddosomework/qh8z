package main

import (
	"html"
	"net/http"
	"strings"

	policies "github.com/gitupofftheflooranddosomework/qh8z/docs"
)

func (a *app) termsPage(w http.ResponseWriter, r *http.Request) {
	a.policyPage(w, r, "Terms of Service", policies.Terms)
}

func (a *app) privacyPage(w http.ResponseWriter, r *http.Request) {
	a.policyPage(w, r, "Privacy Policy", policies.Privacy)
}

func (a *app) acceptableUsePage(w http.ResponseWriter, r *http.Request) {
	a.policyPage(w, r, "Acceptable Use Policy", policies.AcceptableUse)
}

func (a *app) policyPage(w http.ResponseWriter, _ *http.Request, title, source string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(title) + ` · qh8z</title><style>
:root{color-scheme:dark}*{box-sizing:border-box}body{margin:0;background:#090b10;color:#e8edf5;font:16px/1.65 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}a{color:#8ab4ff}header,main,footer{width:min(900px,calc(100% - 32px));margin:auto}header{padding:28px 0 18px;display:flex;align-items:center;justify-content:space-between;gap:20px;border-bottom:1px solid #252b36}.brand{font-size:22px;font-weight:800;text-decoration:none;color:#fff}nav{display:flex;gap:16px;flex-wrap:wrap}nav a,footer a{color:#b9c5d8;text-decoration:none}main{padding:38px 0 56px}article{background:#10141c;border:1px solid #252b36;border-radius:18px;padding:clamp(22px,5vw,52px);box-shadow:0 20px 80px rgba(0,0,0,.25)}h1{font-size:clamp(34px,7vw,54px);line-height:1.05;margin:0 0 28px;color:#fff}h2{font-size:25px;line-height:1.25;margin:38px 0 12px;color:#fff}h3{font-size:19px;margin:28px 0 8px;color:#fff}p{margin:0 0 18px}ul,ol{margin:0 0 20px;padding-left:28px}li{margin:6px 0}strong{color:#fff}code{background:#1b2230;border:1px solid #2c3545;border-radius:6px;padding:1px 5px;font-size:.92em}footer{padding:0 0 44px;color:#7e8a9e;font-size:14px;display:flex;gap:14px;flex-wrap:wrap}@media(max-width:620px){header{align-items:flex-start;flex-direction:column}article{border-radius:14px}}
</style></head><body><header><a class="brand" href="/">qh8z</a><nav><a href="/terms">Terms</a><a href="/privacy">Privacy</a><a href="/acceptable-use">Acceptable Use</a></nav></header><main><article>`))
	_, _ = w.Write([]byte(renderPolicyMarkdown(source)))
	_, _ = w.Write([]byte(`</article></main><footer><span>© qh8z</span><a href="mailto:support@qh8z.com">Support</a><a href="mailto:abuse@qh8z.com">Report abuse</a><a href="mailto:privacy@qh8z.com">Privacy</a><a href="mailto:security@qh8z.com">Security</a></footer></body></html>`))
}

func renderPolicyMarkdown(source string) string {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	var out strings.Builder
	list := ""
	closeList := func() {
		if list != "" {
			out.WriteString("</" + list + ">")
			list = ""
		}
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			closeList()
			continue
		}
		switch {
		case strings.HasPrefix(line, "### "):
			closeList()
			out.WriteString("<h3>" + policyInline(strings.TrimSpace(line[4:])) + "</h3>")
		case strings.HasPrefix(line, "## "):
			closeList()
			out.WriteString("<h2>" + policyInline(strings.TrimSpace(line[3:])) + "</h2>")
		case strings.HasPrefix(line, "# "):
			closeList()
			out.WriteString("<h1>" + policyInline(strings.TrimSpace(line[2:])) + "</h1>")
		case strings.HasPrefix(line, "- "):
			if list != "ul" {
				closeList()
				out.WriteString("<ul>")
				list = "ul"
			}
			out.WriteString("<li>" + policyInline(strings.TrimSpace(line[2:])) + "</li>")
		default:
			closeList()
			out.WriteString("<p>" + policyInline(line) + "</p>")
		}
	}
	closeList()
	return out.String()
}

func policyInline(value string) string {
	value = html.EscapeString(value)
	value = replacePolicyPairs(value, "**", "<strong>", "</strong>")
	value = replacePolicyPairs(value, "`", "<code>", "</code>")
	return value
}

func replacePolicyPairs(value, marker, open, close string) string {
	if strings.Count(value, marker)%2 != 0 {
		return value
	}
	var out strings.Builder
	opening := true
	for {
		index := strings.Index(value, marker)
		if index < 0 {
			out.WriteString(value)
			break
		}
		out.WriteString(value[:index])
		if opening {
			out.WriteString(open)
		} else {
			out.WriteString(close)
		}
		opening = !opening
		value = value[index+len(marker):]
	}
	return out.String()
}
