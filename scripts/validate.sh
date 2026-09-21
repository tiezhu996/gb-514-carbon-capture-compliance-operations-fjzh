#!/usr/bin/env sh
set -eu
project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"
set -a
if [ -f .env ]; then . ./.env; else . ./.env.example; fi
set +a
(command -v jq >/dev/null 2>&1) || { echo "jq is required for API validation" >&2; exit 1; }
(cd backend && go test ./... && go build ./...)
(cd frontend && npm install --no-audit --no-fund && npm run build)
docker compose config --quiet
docker compose up -d --build
cleanup() { docker compose down -v --remove-orphans; }
if [ "${KEEP_RUNNING:-0}" = "1" ]; then
  trap cleanup INT TERM
else
  trap cleanup EXIT INT TERM
fi
i=0
until curl -fsS "http://127.0.0.1:${BACKEND_PORT:-19514}/healthz" >/dev/null; do
  i=$((i+1)); [ "$i" -lt 60 ] || { docker compose logs; exit 1; }; sleep 2
done
curl -fsS "http://127.0.0.1:${FRONTEND_PORT:-18514}/" >/dev/null
token=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT:-19514}/api/auth/login" -H 'Content-Type: application/json' -d '{"username":"admin","password":"Admin123!"}' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
[ -n "$token" ]
viewer_token=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT:-19514}/api/auth/login" -H 'Content-Type: application/json' -d '{"username":"viewer","password":"Admin123!"}' | jq -er '.data.token')
operator_token=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT:-19514}/api/auth/login" -H 'Content-Type: application/json' -d '{"username":"operator","password":"Admin123!"}' | jq -er '.data.token')
reviewer_token=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT:-19514}/api/auth/login" -H 'Content-Type: application/json' -d '{"username":"reviewer","password":"Admin123!"}' | jq -er '.data.token')
curl -fsS "http://127.0.0.1:${BACKEND_PORT:-19514}/api/overview" -H "Authorization: Bearer $token" >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/session" -H "Authorization: Bearer $token" | jq -e '.data.role == "admin" and (.data.requestId | length > 0)' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/runtime" -H "Authorization: Bearer $token" | jq -e '.data.appName and .data.databaseDriver and (.data.requestLimit > 0)' >/dev/null
paths=$(sed -n "s/.*path: '\\([^']*\\)'.*/\\1/p" frontend/src/types/status.ts)
for path in $paths; do
  curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/$path?page=1&pageSize=20" -H "Authorization: Bearer $token" | jq -e '.data | type == "array"' >/dev/null
done
entity_config=$(sed -n "s/.*path: '\\([^']*\\)'.*statuses: \\['\\([^']*\\)', '\\([^']*\\)'.*/\\1|\\2|\\3/p" frontend/src/types/status.ts | head -n 1)
resource=$(printf '%s' "$entity_config" | cut -d '|' -f 1)
initial_status=$(printf '%s' "$entity_config" | cut -d '|' -f 2)
next_status=$(printf '%s' "$entity_config" | cut -d '|' -f 3)
now=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
code="SMOKE-$(date +%s)"
payload=$(printf '{"code":"%s","name":"Runtime smoke record","description":"Automated Compose workflow validation","facility":"Validation Lab","owner":"admin","category":"smoke","riskLevel":"low","metricValue":1,"metricUnit":"unit","effectiveAt":"%s","evidence":"scripts/validate.sh","relatedCode":"SMOKE"}' "$code" "$now")
created=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/$resource" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$payload")
id=$(printf '%s' "$created" | jq -er '.data.id')
version=$(printf '%s' "$created" | jq -er '.data.version')
printf '%s' "$created" | jq -e --arg status "$initial_status" '.data.status == $status' >/dev/null
transition=$(printf '{"status":"%s","expectedVersion":%s,"reason":"automated runtime validation"}' "$next_status" "$version")
curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/$resource/$id/transition" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d "$transition" | jq -e --arg status "$next_status" '.data.status == $status' >/dev/null

viewer_write_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/units" -H "Authorization: Bearer $viewer_token" -H 'Content-Type: application/json' -d "$payload")
[ "$viewer_write_status" = "403" ]
viewer_audit_status=$(curl -sS -o /dev/null -w '%{http_code}' "http://127.0.0.1:${BACKEND_PORT}/api/audits?page=1&pageSize=20" -H "Authorization: Bearer $viewer_token")
[ "$viewer_audit_status" = "403" ]

decision_code="DECISION-SMOKE-$(date +%s)"
decision_payload=$(printf '{"code":"%s","name":"Versioned compliance decision","description":"Immutable evidence workflow validation","facility":"Capture Train A","owner":"operator","category":"emissions","riskLevel":"high","metricValue":31.5,"metricUnit":"ppm","effectiveAt":"%s","evidence":"Calibrated sample ES-SMOKE and permit PR-SMOKE","relatedCode":"PR-SMOKE","unitCode":"CU-SMOKE","sampleCode":"ES-SMOKE"}' "$decision_code" "$now")
decision=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/decisions" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$decision_payload")
decision_id=$(printf '%s' "$decision" | jq -er '.data.id')
decision_version=$(printf '%s' "$decision" | jq -er '.data.version')
printf '%s' "$decision" | jq -e '.data.status == "draft" and (.data.revisions | length) == 1 and .data.revisions[0].version == 1 and .data.revisions[0].actor == "operator" and (.data.revisions[0].requestId | length > 0)' >/dev/null
review_payload=$(printf '{"status":"review","expectedVersion":%s,"reason":"Calibrated evidence package is ready for independent review"}' "$decision_version")
reviewed=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/decisions/$decision_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$review_payload")
review_version=$(printf '%s' "$reviewed" | jq -er '.data.version')
printf '%s' "$reviewed" | jq -e '.data.status == "review" and (.data.revisions | length) == 2 and .data.revisions[0].version == 1 and .data.revisions[1].version == 2 and .data.revisions[1].actor == "operator"' >/dev/null
accept_payload=$(printf '{"status":"accepted","expectedVersion":%s,"reason":"Permit threshold and calibrated evidence agree"}' "$review_version")
operator_accept_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/decisions/$decision_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$accept_payload")
[ "$operator_accept_status" = "422" ]
accepted=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/decisions/$decision_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d "$accept_payload")
printf '%s' "$accepted" | jq -e '.data.status == "accepted" and (.data.revisions | length) == 3 and .data.revisions[0].version == 1 and .data.revisions[1].version == 2 and .data.revisions[2].version == 3 and .data.revisions[2].actor == "reviewer" and (.data.revisions[2].requestId | length > 0)' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audits?page=1&pageSize=100" -H "Authorization: Bearer $token" | jq -e '.meta.total >= 2' >/dev/null
curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/audit-summary?windowHours=24" -H "Authorization: Bearer $token" | jq -e '.data.total >= 2 and .data.transitions >= 1' >/dev/null

# 排放样本作废 -> 已接受决定回退 review_required -> 替代样本终审
void_sample_code="VOID-SAMPLE-$(date +%s)"
void_sample_payload=$(printf '{"code":"%s","name":"Voidable emission sample","facility":"Capture Train A","owner":"operator","category":"emissions","riskLevel":"high","metricValue":30,"metricUnit":"ppm","effectiveAt":"%s","evidence":"pre-void lab report","relatedCode":"","unitCode":"CU-VOID"}' "$void_sample_code" "$now")"
vs=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/samples" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$void_sample_payload")
vs_id=$(printf '%s' "$vs" | jq -er '.data.id'); vs_v=$(printf '%s' "$vs" | jq -er '.data.version')
vs=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/samples/$vs_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$(printf '{"status":"testing","expectedVersion":%s","reason":"send to testing"}' "$vs_v")")
vs_v=$(printf '%s' "$vs" | jq -er '.data.version')
vs=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/samples/$vs_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$(printf '{"status":"verified","expectedVersion":%s,"reason":"verified before use"}' "$vs_v")")
void_decision_code="VOID-DECISION-$(date +%s)"
vd_payload=$(printf '{"code":"%s","name":"Decision citing voided sample","facility":"Capture Train A","owner":"operator","category":"emissions","riskLevel":"high","metricValue":30,"metricUnit":"ppm","effectiveAt":"%s","evidence":"cites voidable sample","relatedCode":"","unitCode":"CU-VOID","sampleCode":"%s"}' "$void_decision_code" "$now" "$void_sample_code")"
vd=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/decisions" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$vd_payload")
vd_id=$(printf '%s' "$vd" | jq -er '.data.id'); vd_v=$(printf '%s' "$vd" | jq -er '.data.version')
vd=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/decisions/$vd_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$(printf '{"status":"review","expectedVersion":%s,"reason":"ready for review"}' "$vd_v")")
vd_v=$(printf '%s' "$vd" | jq -er '.data.version')
vd=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/decisions/$vd_id/transition" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d "$(printf '{"status":"accepted","expectedVersion":%s","reason":"accept citing sample"}' "$vd_v")")
printf '%s' "$vd" | jq -e '.data.status == "accepted"' >/dev/null
# 作废样本必须回退决定；重复作废必须 422 且不改动记录
voided=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/samples/$vs_id/void" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d '{"reason":"sampler calibration failed"}')
printf '%s' "$voided" | jq -e '.data.status == "invalid" and (.data.voidedAt | length > 0) and (.data.affectedDecisions | length) == 1 and .data.affectedDecisions[0].decisionCode == $code and .data.affectedDecisions[0].previousState == "accepted"' --arg code "$void_decision_code" >/dev/null
repeat_void_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/samples/$vs_id/void" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d '{"reason":"repeated void"}')
[ "$repeat_void_status" = "422" ]
rolled=$(curl -fsS "http://127.0.0.1:${BACKEND_PORT}/api/decisions/$vd_id/rollback" -H "Authorization: Bearer $reviewer_token")
printf '%s' "$rolled" | jq -e '.data.status == "review_required" and .data.rollback.voidedSampleCode == $code and .data.rollback.voidReason == "sampler calibration failed" and (.data.revisions[0].evidence | length > 0)' --arg code "$void_sample_code" >/dev/null
# operator 不能终审
op_finalize_status=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:${BACKEND_PORT}/api/decisions/$vd_id/finalize" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d '{"substituteSampleId":1,"reason":"operator tries final review"}')
[ "$op_finalize_status" = "403" ]
# 同装置、已验证、采样晚于作废时间的替代样本终审
sub_time=$(date -u -d '+2 hours' '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u -v+2H '+%Y-%m-%dT%H:%M:%SZ')
sub_code="SUB-SAMPLE-$(date +%s)"
sub_payload=$(printf '{"code":"%s","name":"Eligible substitute sample","facility":"Capture Train A","owner":"reviewer","category":"emissions","riskLevel":"high","metricValue":28,"metricUnit":"ppm","effectiveAt":"%s","evidence":"post-void verified lab report","relatedCode":"","unitCode":"CU-VOID"}' "$sub_code" "$sub_time")
ss=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/samples" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$sub_payload")
ss_id=$(printf '%s' "$ss" | jq -er '.data.id'); ss_v=$(printf '%s' "$ss" | jq -er '.data.version')
ss=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/samples/$ss_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$(printf '{"status":"testing","expectedVersion":%s,"reason":"substitute testing"}' "$ss_v")")
ss_v=$(printf '%s' "$ss" | jq -er '.data.version')
ss=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/samples/$ss_id/transition" -H "Authorization: Bearer $operator_token" -H 'Content-Type: application/json' -d "$(printf '{"status":"verified","expectedVersion":%s,"reason":"substitute verified"}' "$ss_v")")
finalized=$(curl -fsS -X POST "http://127.0.0.1:${BACKEND_PORT}/api/decisions/$vd_id/finalize" -H "Authorization: Bearer $reviewer_token" -H 'Content-Type: application/json' -d "$(printf '{"substituteSampleId":%s,"reason":"substitute confirms compliance"}' "$ss_id")")
printf '%s' "$finalized" | jq -e '.data.status == "accepted" and .data.sampleCode == $code and .data.rollback.substituteCode == $code and (.data.rollback.finalizedAt | length > 0)' --arg code "$sub_code" >/dev/null

docker compose ps
if [ "${KEEP_RUNNING:-0}" = "1" ]; then
  echo "KEEP_RUNNING=1: containers left running for browser validation"
fi
