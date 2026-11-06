package ydsz

import "context"

// SearchRequest 日志检索请求。
type SearchRequest struct {
	Item     int    `json:"item"`
	Date     string `json:"date"`
	Key      string `json:"key,omitempty"`
	Query    string `json:"query,omitempty"`
	Regex    bool   `json:"regex,omitempty"`
	Level    string `json:"level,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	EndTime  string `json:"endTime,omitempty"`
	Client   int64  `json:"client,omitempty"`
	PageNo   int    `json:"pageNo,omitempty"`
	PageSize int    `json:"pageSize,omitempty"`
}

// SearchResponse 日志检索响应。
type SearchResponse struct {
	Lines    []string `json:"lines"`
	Total    int64    `json:"total"`
	PageNo   int      `json:"pageNo"`
	PageSize int      `json:"pageSize"`
}

// ContextRequest 上下文回溯请求。
type ContextRequest struct {
	RefTime string `json:"refTime,omitempty"`
	RefLine string `json:"refLine,omitempty"`
	Before  int    `json:"before,omitempty"`
	After   int    `json:"after,omitempty"`
	Date    string `json:"date,omitempty"`
}

// ContextResponse 上下文回溯响应。
type ContextResponse struct {
	Before []string `json:"before"`
	Match  string   `json:"match"`
	After  []string `json:"after"`
}

// TraceRequest 链路查询请求。
type TraceRequest struct {
	Item     int    `json:"item"`
	Date     string `json:"date"`
	TraceID  string `json:"traceId"`
	Client   int64  `json:"client,omitempty"`
}

// TraceResponse 链路查询响应。
type TraceResponse struct {
	TraceID string   `json:"traceId"`
	Lines   []string `json:"lines"`
}

// Nl2DslRequest 自然语言解析请求。
type Nl2DslRequest struct {
	Query string `json:"query"`
	ConvID string `json:"convId,omitempty"`
}

// Nl2DslResponse 自然语言解析响应。
type Nl2DslResponse struct {
	UserIntent      string   `json:"userIntent"`
	Level           string   `json:"level"`
	Date            string   `json:"date"`
	StartTime       string   `json:"startTime"`
	EndTime         string   `json:"endTime"`
	Key             string   `json:"key"`
	TimeRangeDetail string   `json:"timeRangeDetail"`
	IntentType      string   `json:"intentType"`
	DslConditions   string   `json:"dslConditions"`
	Strategy        string   `json:"strategy"`
	Confidence      float64  `json:"confidence"`
}

// SemanticSearchRequest 语义检索请求。
type SemanticSearchRequest struct {
	Query string `json:"query"`
	Type  string `json:"type,omitempty"`
	TopK  int    `json:"topK,omitempty"`
}

// SemanticResponse 语义检索响应。
type SemanticResponse struct {
	Results  []SemanticResult `json:"results"`
	Degraded bool             `json:"degraded"`
}

// SemanticResult 单条语义召回结果。
type SemanticResult struct {
	TargetID   string                 `json:"targetId"`
	TargetType string                 `json:"targetType"`
	Score      float64                `json:"score"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// TemplateResponse 模板列表响应。
type TemplateResponse struct {
	Templates []TemplateItem `json:"templates"`
	Total     int            `json:"total"`
}

// TemplateItem 单条模板。
type TemplateItem struct {
	TemplateHash string `json:"templateHash"`
	Sample       string `json:"sample"`
	Count        int    `json:"count"`
	Level        string `json:"level"`
}

// AnomalyResponse 异常事件响应。
type AnomalyResponse struct {
	Points []AnomalyPoint `json:"points"`
	Total  int            `json:"total"`
}

// AnomalyPoint 单条异常事件。
type AnomalyPoint struct {
	Time         string  `json:"time"`
	TemplateHash string  `json:"templateHash"`
	Type         string  `json:"type"`
	Score        float64 `json:"score"`
	Description  string  `json:"description"`
}

// ReasonRequest 根因推理请求。
type ReasonRequest struct {
	Query string `json:"query"`
	Item  int    `json:"item,omitempty"`
}

// ReasonResponse 根因推理响应。
type ReasonResponse struct {
	Hypothesis   string          `json:"hypothesis"`
	Conclusion   ReasonConclusion `json:"conclusion"`
	ToolCalls    []ToolCallRecord `json:"toolCalls"`
	Evidence     []EvidenceSection `json:"evidence"`
	Degraded     bool            `json:"degraded"`
	ModelName    string          `json:"modelName"`
	TotalTokens  int             `json:"totalTokens"`
	MaxSteps     int             `json:"maxSteps"`
}

// ReasonConclusion 结论段。
type ReasonConclusion struct {
	RootCause   string   `json:"rootCause"`
	Suggestions []string `json:"suggestions"`
	Confidence  float64  `json:"confidence"`
}

// ToolCallRecord 工具调用记录。
type ToolCallRecord struct {
	Step   int    `json:"step"`
	Tool   string `json:"tool"`
	Params map[string]interface{} `json:"params"`
	ResultSummary string `json:"resultSummary"`
	DurationMs    int64  `json:"durationMs"`
	Success       bool   `json:"success"`
}

// EvidenceSection 证据段。
type EvidenceSection struct {
	Title string   `json:"title"`
	Refs  []string `json:"refs"`
}

// ─── API 方法 ───

// SearchLogs 按条件检索日志。
func (c *Client) SearchLogs(ctx context.Context, req *SearchRequest) (*SearchResponse, error) {
	var out SearchResponse
	if err := c.do(ctx, "POST", "/logs/search", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetContext 上下文回溯。
func (c *Client) GetContext(ctx context.Context, itemID int, req *ContextRequest) (*ContextResponse, error) {
	var out ContextResponse
	path := "/logs/context/" + itoa(itemID)
	if err := c.do(ctx, "GET", path, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTrace 链路查询。
func (c *Client) GetTrace(ctx context.Context, req *TraceRequest) (*TraceResponse, error) {
	var out TraceResponse
	if err := c.do(ctx, "POST", "/logs/trace", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Nl2DslParse 自然语言解析。
func (c *Client) Nl2DslParse(ctx context.Context, req *Nl2DslRequest) (*Nl2DslResponse, error) {
	var out Nl2DslResponse
	if err := c.do(ctx, "POST", "/logs/nl2dsl/parse", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SemanticSearch 语义检索。
func (c *Client) SemanticSearch(ctx context.Context, req *SemanticSearchRequest) (*SemanticResponse, error) {
	var out SemanticResponse
	if err := c.do(ctx, "POST", "/api/v2/semantic/search", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTemplates 模板列表。
func (c *Client) ListTemplates(ctx context.Context, timeRange string, topN int) (*TemplateResponse, error) {
	req := map[string]interface{}{"timeRange": timeRange, "topN": topN}
	var out TemplateResponse
	if err := c.do(ctx, "GET", "/logs/templates/search", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAnomalies 异常事件查询。
func (c *Client) GetAnomalies(ctx context.Context, timeRange string, limit int) (*AnomalyResponse, error) {
	req := map[string]interface{}{"timeRange": timeRange, "limit": limit}
	var out AnomalyResponse
	if err := c.do(ctx, "GET", "/logs/anomalies/timeline", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ReasonIncident 根因推理。
func (c *Client) ReasonIncident(ctx context.Context, req *ReasonRequest) (*ReasonResponse, error) {
	var out ReasonResponse
	if err := c.do(ctx, "POST", "/logs/agent/reason", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
