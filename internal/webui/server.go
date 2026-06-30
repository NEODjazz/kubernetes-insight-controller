package webui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s-insight-controller/api/v1alpha1"
)

type Server struct {
	Client client.Client
	Addr   string
}

type reportResponse struct {
	Namespace       string `json:"namespace"`
	Name            string `json:"name"`
	Ready           bool   `json:"ready"`
	Reason          string `json:"reason"`
	ObservedAt      string `json:"observedAt,omitempty"`
	AzureDeployment string `json:"azureDeployment"`
	RetentionDays   int    `json:"retentionDays"`
}

type snapshotResponse struct {
	Namespace         string `json:"namespace"`
	Name              string `json:"name"`
	InsightReportName string `json:"insightReportName"`
	AnalyzedAt        string `json:"analyzedAt"`
	DurationMillis    int64  `json:"durationMillis"`
	Recommendations   string `json:"recommendations"`
}

func (s *Server) Start(ctx context.Context) error {
	addr := s.Addr
	if addr == "" {
		addr = ":8090"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/api/reports", s.reports)
	mux.HandleFunc("/api/snapshots", s.snapshots)

	server := &http.Server{Addr: addr, Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		log.FromContext(ctx).Info("starting web ui", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var reports v1alpha1.InsightReportList
	if err := s.Client.List(r.Context(), &reports); err != nil {
		http.Error(w, fmt.Sprintf("list insightreports: %v", err), http.StatusInternalServerError)
		return
	}

	items := make([]reportResponse, 0, len(reports.Items))
	for _, report := range reports.Items {
		ready, reason := reportCondition(report)
		observedAt := ""
		if report.Status.ObservedAt != nil {
			observedAt = report.Status.ObservedAt.Time.Format(time.RFC3339)
		}
		items = append(items, reportResponse{
			Namespace:       report.Namespace,
			Name:            report.Name,
			Ready:           ready,
			Reason:          reason,
			ObservedAt:      observedAt,
			AzureDeployment: report.Spec.AzureDeployment,
			RetentionDays:   effectiveRetentionDays(report.Spec.RetentionDays),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Namespace+"/"+items[i].Name < items[j].Namespace+"/"+items[j].Name
	})
	writeJSON(w, map[string]any{"items": items})
}

func (s *Server) snapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reportFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("report")))
	snapshotFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("snapshot")))
	textFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	var snapshots v1alpha1.InsightReportSnapshotList
	if err := s.Client.List(r.Context(), &snapshots); err != nil {
		http.Error(w, fmt.Sprintf("list insightreportsnapshots: %v", err), http.StatusInternalServerError)
		return
	}

	items := make([]snapshotResponse, 0, len(snapshots.Items))
	for _, snapshot := range snapshots.Items {
		reportKey := strings.ToLower(snapshot.Namespace + "/" + snapshot.Spec.InsightReportName)
		if reportFilter != "" && reportKey != reportFilter && strings.ToLower(snapshot.Spec.InsightReportName) != reportFilter {
			continue
		}
		if snapshotFilter != "" && !strings.Contains(strings.ToLower(snapshot.Name), snapshotFilter) {
			continue
		}
		if textFilter != "" && !strings.Contains(strings.ToLower(snapshot.Spec.Recommendations), textFilter) {
			continue
		}
		items = append(items, snapshotResponse{
			Namespace:         snapshot.Namespace,
			Name:              snapshot.Name,
			InsightReportName: snapshot.Spec.InsightReportName,
			AnalyzedAt:        snapshot.Spec.AnalyzedAt.Time.Format(time.RFC3339),
			DurationMillis:    snapshot.Spec.DurationMillis,
			Recommendations:   snapshot.Spec.Recommendations,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].AnalyzedAt > items[j].AnalyzedAt
	})
	writeJSON(w, map[string]any{"items": items})
}

func reportCondition(report v1alpha1.InsightReport) (bool, string) {
	for _, condition := range report.Status.Conditions {
		if condition.Type == "Ready" {
			return condition.Status == "True", condition.Reason
		}
	}
	return false, "Unknown"
}

func effectiveRetentionDays(value int) int {
	if value < 1 {
		return 30
	}
	return value
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

const indexHTML = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Kubernetes Insight History</title>
  <style>
    :root { --bg:#f5f7fa; --panel:#fff; --line:#d9dee7; --text:#182230; --muted:#657083; --accent:#1769e0; --ok:#137a4d; }
    * { box-sizing:border-box; }
    body { margin:0; min-height:100vh; color:var(--text); background:var(--bg); font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; }
    header { min-height:58px; display:flex; align-items:center; justify-content:space-between; gap:16px; padding:10px 22px; background:var(--panel); border-bottom:1px solid var(--line); }
    h1 { margin:0; font-size:17px; letter-spacing:0; }
    button,select,input { min-height:36px; border:1px solid var(--line); border-radius:6px; background:#fff; color:var(--text); font:inherit; }
    button { padding:0 14px; cursor:pointer; }
    select,input { width:100%; padding:0 10px; }
    .filters { display:grid; grid-template-columns:minmax(190px,1fr) minmax(180px,1fr) minmax(240px,2fr); gap:10px; padding:12px 16px; background:var(--panel); border-bottom:1px solid var(--line); }
    main { display:grid; grid-template-columns:minmax(300px,380px) minmax(0,1fr); min-height:calc(100vh - 119px); }
    aside { overflow:auto; border-right:1px solid var(--line); background:#fbfcfd; }
    .row { width:100%; display:grid; gap:5px; padding:13px 14px; border:0; border-bottom:1px solid var(--line); border-radius:0; text-align:left; background:transparent; }
    .row[aria-selected="true"] { background:#edf4ff; box-shadow:inset 3px 0 var(--accent); }
    .row-title { display:flex; justify-content:space-between; gap:8px; font-weight:700; overflow-wrap:anywhere; }
    .meta,.label { color:var(--muted); font-size:12px; overflow-wrap:anywhere; }
    .duration { color:var(--ok); white-space:nowrap; font-size:12px; }
    section { min-width:0; overflow:auto; padding:20px 24px 34px; }
    .metrics { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:12px; margin-bottom:16px; }
    .metric,.panel { background:var(--panel); border:1px solid var(--line); border-radius:8px; }
    .metric { min-height:76px; padding:12px; }
    .value { margin-top:7px; font-size:16px; font-weight:700; overflow-wrap:anywhere; }
    .panel h2 { margin:0; padding:13px 16px; border-bottom:1px solid var(--line); font-size:14px; letter-spacing:0; }
    .recommendations { padding:16px; overflow-wrap:anywhere; }
    .recommendations > :first-child { margin-top:0; }
    .recommendations > :last-child { margin-bottom:0; }
    .recommendations h1,.recommendations h2,.recommendations h3,.recommendations h4,.recommendations h5,.recommendations h6 { margin:1.25em 0 .55em; line-height:1.25; }
    .recommendations h1 { font-size:1.65em; } .recommendations h2 { font-size:1.4em; } .recommendations h3 { font-size:1.2em; }
    .recommendations p,.recommendations ul,.recommendations ol,.recommendations blockquote,.recommendations pre,.recommendations .table-wrap,.recommendations .code-block { margin:.75em 0; }
    .recommendations ul,.recommendations ol { padding-left:1.6em; }
    .recommendations blockquote { padding:.1em 1em; border-left:3px solid var(--line); color:var(--muted); }
    .recommendations code { padding:.15em .35em; border-radius:4px; background:#eef1f5; font:13px/1.5 ui-monospace,SFMono-Regular,Consolas,monospace; }
    .recommendations .code-block { overflow:hidden; border:1px solid var(--line); border-radius:6px; background:#f7f8fa; }
    .recommendations .code-language { padding:5px 12px; border-bottom:1px solid var(--line); background:#eef1f5; color:var(--muted); font:11px/1.5 ui-monospace,SFMono-Regular,Consolas,monospace; text-transform:uppercase; }
    .recommendations pre { overflow:auto; padding:12px 14px; border:1px solid var(--line); border-radius:6px; background:#f7f8fa; }
    .recommendations .code-block pre { margin:0; border:0; border-radius:0; }
    .recommendations pre code { padding:0; background:transparent; white-space:pre; }
    .recommendations .table-wrap { overflow-x:auto; border:1px solid var(--line); border-radius:6px; }
    .recommendations table { width:100%; border-collapse:collapse; background:var(--panel); }
    .recommendations th,.recommendations td { padding:8px 10px; border-right:1px solid var(--line); border-bottom:1px solid var(--line); text-align:left; vertical-align:top; }
    .recommendations th { background:#f3f5f8; font-weight:700; }
    .recommendations tr:last-child td { border-bottom:0; }
    .recommendations th:last-child,.recommendations td:last-child { border-right:0; }
    .recommendations .align-center { text-align:center; } .recommendations .align-right { text-align:right; }
    .recommendations a { color:var(--accent); }
    .recommendations hr { border:0; border-top:1px solid var(--line); margin:1.25em 0; }
    .empty { padding:36px; text-align:center; color:var(--muted); }
    @media(max-width:900px) { .filters{grid-template-columns:1fr;} main{grid-template-columns:1fr;} aside{max-height:40vh;border-right:0;border-bottom:1px solid var(--line);} section{padding:14px;} .metrics{grid-template-columns:repeat(2,minmax(0,1fr));} }
  </style>
</head>
<body>
  <header><h1>Kubernetes Insight History</h1><button id="refresh" title="Обновить данные">Обновить</button></header>
  <div class="filters">
    <select id="report"><option value="">Все InsightReport</option></select>
    <input id="snapshot" type="search" placeholder="Имя InsightReportSnapshot">
    <input id="text" type="search" placeholder="Текст внутри рекомендации">
  </div>
  <main>
    <aside id="list"></aside>
    <section id="content" class="empty">Нет снапшотов</section>
  </main>
  <script>
    const state={reports:[],snapshots:[],selected:""};
    const report=document.getElementById("report"), snapshot=document.getElementById("snapshot"), text=document.getElementById("text");
    const list=document.getElementById("list"), content=document.getElementById("content"), refresh=document.getElementById("refresh");
    const esc=v=>String(v??"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"}[c]));
    const key=s=>s.namespace+"/"+s.name;
    const duration=ms=>ms<1000?ms+" ms":ms<60000?(ms/1000).toFixed(1)+" s":(ms/60000).toFixed(1)+" min";
    const filtered=()=>state.snapshots.filter(s=>(!report.value||s.namespace+"/"+s.insightReportName===report.value)&&(!snapshot.value||s.name.toLowerCase().includes(snapshot.value.toLowerCase()))&&(!text.value||s.recommendations.toLowerCase().includes(text.value.toLowerCase())));
    function inlineMarkdown(value){
      let source=esc(value),codes=[];
      source=source.replace(new RegExp("\\x60([^\\x60\\n]+)\\x60","g"),(_,code)=>{codes.push("<code>"+code+"</code>");return "\u0000"+(codes.length-1)+"\u0000";});
      source=source.replace(/\[([^\]\n]+)\]\(([^)\s]+)\)/g,(_,label,url)=>{
        const decoded=url.replace(/&amp;/g,"&").replace(/&#39;/g,"'").replace(/&quot;/g,'"');
        return /^(https?:|mailto:|#|\/)/i.test(decoded)?'<a href="'+url+'" target="_blank" rel="noopener noreferrer">'+label+"</a>":label;
      });
      source=source.replace(/\*\*([^*\n]+)\*\*/g,"<strong>$1</strong>").replace(/__([^_\n]+)__/g,"<strong>$1</strong>");
      source=source.replace(/(^|[\s(])\*([^*\n]+)\*(?=$|[\s).,!?:;])/g,"$1<em>$2</em>").replace(/(^|[\s(])_([^_\n]+)_(?=$|[\s).,!?:;])/g,"$1<em>$2</em>");
      source=source.replace(/\u0000(\d+)\u0000/g,(_,i)=>codes[Number(i)]);
      return source;
    }
    function renderCodeBlock(code,language){
      const label=language?'<div class="code-language">'+esc(language)+'</div>':"";
      return '<div class="code-block">'+label+'<pre><code'+(language?' class="language-'+esc(language)+'"':"")+">"+esc(code.join("\n"))+"</code></pre></div>";
    }
    function splitTableRow(line){
      const source=line.trim().replace(/^\|/,"").replace(/\|$/,""),cells=[];
      let cell="",escaped=false;
      for(const char of source){
        if(escaped){cell+=char;escaped=false;continue;}
        if(char==="\\"){escaped=true;continue;}
        if(char==="|"){cells.push(cell.trim());cell="";continue;}
        cell+=char;
      }
      cells.push(cell.trim());
      return cells;
    }
    function tableAlignments(line){
      const cells=splitTableRow(line);
      if(!cells.length||cells.some(cell=>!/^:?-{3,}:?$/.test(cell)))return null;
      return cells.map(cell=>cell.startsWith(":")&&cell.endsWith(":")?"center":cell.endsWith(":")?"right":"left");
    }
    function renderTable(header,alignments,rows){
      const className=alignment=>alignment==="center"?"align-center":alignment==="right"?"align-right":"";
      const cells=(values,tag)=>values.map((value,index)=>"<"+tag+' class="'+className(alignments[index]||"left")+'">'+inlineMarkdown(value)+"</"+tag+">").join("");
      return '<div class="table-wrap"><table><thead><tr>'+cells(header,"th")+"</tr></thead><tbody>"+rows.map(row=>"<tr>"+cells(row,"td")+"</tr>").join("")+"</tbody></table></div>";
    }
    function renderMarkdown(value){
      const lines=String(value??"").replace(/\r\n?/g,"\n").split("\n"),out=[];
      let paragraph=[],listType="",listItems=[],quote=[],code=[],codeLanguage="",codeIndent=0,inCode=false;
      const flushParagraph=()=>{if(paragraph.length){out.push("<p>"+inlineMarkdown(paragraph.join("\n")).replace(/\n/g,"<br>")+"</p>");paragraph=[];}};
      const flushList=()=>{if(listItems.length){out.push("<"+listType+">"+listItems.map(item=>"<li>"+inlineMarkdown(item)+"</li>").join("")+"</"+listType+">");listItems=[];listType="";}};
      const flushQuote=()=>{if(quote.length){out.push("<blockquote>"+renderMarkdown(quote.join("\n"))+"</blockquote>");quote=[];}};
      const flushAll=()=>{flushParagraph();flushList();flushQuote();};
      for(let index=0;index<lines.length;index++){
        const line=lines[index];
        const fence=line.match(new RegExp("^( {0,3})\\x60{3,}([^\\s\\x60]*)\\s*$"));
        if(fence){
          if(inCode){out.push(renderCodeBlock(code,codeLanguage));code=[];codeLanguage="";codeIndent=0;inCode=false;}
          else{flushAll();inCode=true;codeIndent=fence[1].length;codeLanguage=fence[2]||"";}
          continue;
        }
        if(inCode){code.push(line.slice(Math.min(codeIndent,line.search(/\S|$/))));continue;}
        const alignments=index+1<lines.length?tableAlignments(lines[index+1]):null;
        if(line.includes("|")&&alignments){
          flushAll();
          const header=splitTableRow(line),rows=[];
          index+=2;
          while(index<lines.length&&lines[index].trim()&&lines[index].includes("|")){rows.push(splitTableRow(lines[index]));index++;}
          index--;
          out.push(renderTable(header,alignments,rows));
          continue;
        }
        const heading=line.match(/^(#{1,6})\s+(.+)$/),unordered=line.match(/^\s*[-+*]\s+(.+)$/),ordered=line.match(/^\s*\d+[.)]\s+(.+)$/),quoted=line.match(/^\s*>\s?(.*)$/);
        if(!line.trim()){flushAll();continue;}
        if(heading){flushAll();const level=heading[1].length;out.push("<h"+level+">"+inlineMarkdown(heading[2])+"</h"+level+">");continue;}
        if(/^(\s*)([-*_])(?:\s*\2){2,}\s*$/.test(line)){flushAll();out.push("<hr>");continue;}
        if(unordered||ordered){flushParagraph();flushQuote();const type=unordered?"ul":"ol";if(listType&&listType!==type)flushList();listType=type;listItems.push((unordered||ordered)[1]);continue;}
        if(quoted){flushParagraph();flushList();quote.push(quoted[1]);continue;}
        flushList();flushQuote();paragraph.push(line);
      }
      if(inCode)out.push(renderCodeBlock(code,codeLanguage));
      flushAll();
      return out.join("");
    }
    function renderList(){
      const items=filtered();
      if(!items.length){list.innerHTML='<div class="empty">Снапшоты не найдены</div>';renderContent();return;}
      if(!items.some(s=>key(s)===state.selected))state.selected=key(items[0]);
      list.innerHTML=items.map(s=>'<button class="row" data-key="'+esc(key(s))+'" aria-selected="'+(key(s)===state.selected)+'"><div class="row-title"><span>'+esc(s.name)+'</span><span class="duration">'+esc(duration(s.durationMillis))+'</span></div><div class="meta">'+esc(s.namespace+"/"+s.insightReportName)+'</div><div class="meta">'+esc(new Date(s.analyzedAt).toLocaleString())+'</div></button>').join("");
      renderContent();
    }
    function renderContent(){
      const s=filtered().find(x=>key(x)===state.selected);
      if(!s){content.className="empty";content.innerHTML="Нет снапшотов";return;}
      content.className="";
      content.innerHTML='<div class="metrics"><div class="metric"><div class="label">InsightReport</div><div class="value">'+esc(s.insightReportName)+'</div></div><div class="metric"><div class="label">Snapshot</div><div class="value">'+esc(s.name)+'</div></div><div class="metric"><div class="label">Дата анализа</div><div class="value">'+esc(new Date(s.analyzedAt).toLocaleString())+'</div></div><div class="metric"><div class="label">Продолжительность</div><div class="value">'+esc(duration(s.durationMillis))+'</div></div></div><div class="panel"><h2>Рекомендации</h2><div class="recommendations">'+renderMarkdown(s.recommendations||"Рекомендации отсутствуют")+'</div></div>';
    }
    async function load(){
      refresh.disabled=true;
      try{
        const [rr,sr]=await Promise.all([fetch("/api/reports",{cache:"no-store"}),fetch("/api/snapshots",{cache:"no-store"})]);
        if(!rr.ok||!sr.ok)throw new Error("Не удалось загрузить данные");
        state.reports=(await rr.json()).items||[];state.snapshots=(await sr.json()).items||[];
        const current=report.value;
        report.innerHTML='<option value="">Все InsightReport</option>'+state.reports.map(r=>{const value=r.namespace+"/"+r.name;return '<option value="'+esc(value)+'" '+(value===current?"selected":"")+'>'+esc(value)+" · "+esc(r.retentionDays)+" дней</option>";}).join("");
        renderList();
      }catch(e){content.className="empty";content.innerHTML=esc(e.message||e);}finally{refresh.disabled=false;}
    }
    list.addEventListener("click",e=>{const row=e.target.closest(".row");if(!row)return;state.selected=row.dataset.key;renderList();});
    [report,snapshot,text].forEach(control=>control.addEventListener("input",renderList));
    refresh.addEventListener("click",load);load();setInterval(load,30000);
  </script>
</body>
</html>`
