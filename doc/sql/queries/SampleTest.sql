-- Params: user_id
-- Returns: total_storage_used reached_file_limit exceeded_storage_limit
-- ResultMode: one
WITH selected_user_plan AS (
    SELECT up.user_id, up.plan_id
    FROM user_plan up
             JOIN plan p ON p.id = up.plan_id
    WHERE up.user_id = ?
    ORDER BY p.price DESC
    LIMIT 1
)
SELECT
    COALESCE(SUM(f.size_bytes), 0) AS total_storage_used,
    (COUNT(f.id) >= p.max_file_count) AS reached_file_limit,
    (COALESCE(SUM(f.size_bytes), 0) >= p.max_storage_bytes) AS exceeded_storage_limit
FROM selected_user_plan sp
         JOIN plan p ON p.id = sp.plan_id
         LEFT JOIN file f ON f.user_id = sp.user_id
GROUP BY sp.user_id, p.max_file_count, p.max_storage_bytes