Bash
#!/bin/bash

# ===================== 配置参数（替换为你的实际值）=====================
# 请求URL
API_URL="https://funcloud.ai/v1/official/chat/completions"
API_KEY="eyJ1c2VyX2lkIjo0NDUsImV4cGlyZXMiOjE5MzEzMTg5NzYsImlzc3VlZF9hdCI6MTc3NTc5ODk3NiwiZXh0cmEiOm51bGx9.QgAfZYDaW4i65jprrf24-HQH2J_2UXh-kW8kIE7TZH4="


# JSON 请求体（根据接口字段修改，注意JSON格式正确）
REQUEST_BODY=$(cat <<EOF
{
    "model": "global.anthropic.claude-opus-4-6-v1",
    "stream": true,
    "messages": [
        {
            "role": "user",
            "content": [
                {
                    "type": "text",
                    "text": "你是谁"
                }
            ]
        }
    ]
}
EOF
)

# ===================== curl 请求执行 =====================
echo "正在发送请求到: $API_URL"
echo "请求体: $REQUEST_BODY"
echo "--------------------------------------------------------"

curl -X POST \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $API_KEY" \
  -d "$REQUEST_BODY" \
  "$API_URL"