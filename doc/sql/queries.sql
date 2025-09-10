-- Name: GetAllAnimals
-- Returns: Animal BigNumber
-- ResultMode: many
SELECT `Animal`, `BigNumber`
FROM `alpha`;

-- Name: GetRecentCats
-- Returns: Uuid LastUpdate
-- ResultMode: many
SELECT `Uuid`, `LastUpdate`
FROM `alpha`
WHERE `Animal` = 'cat' AND `LastUpdate` > '2024-01-01 00:00:00.000000';

-- Name: GetByUuid
-- Params: uuid
-- Returns: Animal test_field
-- ResultMode: one
SELECT `Animal`, `test_field`
FROM `alpha`
WHERE `Uuid` = ?;

-- Name: CountNullBigNumbers
-- Returns: count
-- ResultMode: one
SELECT COUNT(*) AS `count`
FROM `alpha`
WHERE `BigNumber` IS NULL;

-- Name: InsertOne
-- Params: uuid animal test_field
-- ResultMode: exec
INSERT INTO `alpha` (`Uuid`, `Animal`, `test_field`)
VALUES (?, ?, ?);

-- Name: InsertHardcoded
-- ResultMode: exec
INSERT INTO `alpha` (`Uuid`, `Animal`)
VALUES ('11111111-1111-4111-8111-111111111111', 'dog');

-- Name: UpdateAnimalName
-- Params: animal uuid
-- ResultMode: exec
UPDATE `alpha`
SET `Animal` = ?
WHERE `Uuid` = ?;

-- Name: UpdateTestField
-- ResultMode: exec
UPDATE `alpha`
SET `test_field` = 'updated'
WHERE `Animal` = 'fox';

-- Name: DeleteByUuid
-- Params: uuid
-- ResultMode: exec
DELETE FROM `alpha` WHERE `Uuid` = ?;

-- Name: DeleteOldRows
-- ResultMode: exec
DELETE FROM `alpha` WHERE `LastUpdate` < '2023-01-01 00:00:00.000000';

-- Name: SampleTest
-- Params: user_id
-- Returns: total_storage_used reached_file_limit exceeded_storage_limit
-- ResultMode: one
-- Transaction
-- Context
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
GROUP BY sp.user_id, p.max_file_count, p.max_storage_bytes;
