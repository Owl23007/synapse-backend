// model/tool.go
package model

// Tool 工具结构
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolFunction 工具函数结构
type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

var ScheduleTools = []Tool{
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "create_schedule",
			Description: "创建一个符合 RFC 5545 标准的日程事件（VEVENT）",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"uid":         map[string]string{"type": "string", "description": "全局唯一标识符，如 '123456789@example.com'"},
					"summary":     map[string]string{"type": "string", "description": "事件标题（SUMMARY）"},
					"dtstart":     map[string]string{"type": "string", "description": "开始时间，格式：YYYYMMDDTHHMMSSZ 或 YYYYMMDDTHHMMSS"},
					"dtend":       map[string]string{"type": "string", "description": "结束时间，格式同上"},
					"description": map[string]string{"type": "string", "description": "事件描述（DESCRIPTION，可选）"},
					"location":    map[string]string{"type": "string", "description": "地点（LOCATION，可选）"},
				},
				"required": []string{"uid", "summary", "dtstart", "dtend"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "get_schedule",
			Description: "根据 UID 或时间范围查询日程事件",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"uid":     map[string]string{"type": "string", "description": "事件唯一ID"},
					"dtstart": map[string]string{"type": "string", "description": "起始时间（用于范围查询）"},
					"dtend":   map[string]string{"type": "string", "description": "结束时间（用于范围查询）"},
				},
				"required": []string{}, // 至少提供一个条件
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "update_schedule",
			Description: "更新已有日程事件（必须提供 uid）",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"uid":         map[string]string{"type": "string", "description": "要更新的事件 UID"},
					"summary":     map[string]string{"type": "string", "description": "新标题（可选）"},
					"dtstart":     map[string]string{"type": "string", "description": "新开始时间（可选）"},
					"dtend":       map[string]string{"type": "string", "description": "新结束时间（可选）"},
					"description": map[string]string{"type": "string", "description": "新描述（可选）"},
					"location":    map[string]string{"type": "string", "description": "新地点（可选）"},
				},
				"required": []string{"uid"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "delete_schedule",
			Description: "删除指定 UID 的日程事件",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"uid": map[string]string{"type": "string", "description": "要删除的事件 UID"},
				},
				"required": []string{"uid"},
			},
		},
	},
}