-- name: CreateStatusHistory :one
INSERT INTO status_history (
    entity_type, entity_id, from_status, to_status, actor_user_id, actor_role, comment
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
) RETURNING *;

-- name: ListApplicationHistory :many
-- История заявки и её офферов. Доступ к заявке проверяется до вызова (GetApplicationVisible);
-- фильтр по офферам не даёт кредитору увидеть историю чужих офферов.
SELECT h.* FROM status_history h
WHERE (h.entity_type = 'application' AND h.entity_id = sqlc.arg('application_id'))
   OR (h.entity_type = 'offer' AND h.entity_id IN (
         SELECT o.id FROM offers o
          WHERE o.application_id = sqlc.arg('application_id')
            AND (sqlc.arg('viewer_role')::user_role <> 'lender'
                 OR o.lender_organization_id = sqlc.arg('viewer_org')::uuid)))
ORDER BY h.created_at, h.id;
