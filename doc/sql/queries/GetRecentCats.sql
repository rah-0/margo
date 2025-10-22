-- Returns: Uuid LastUpdate
-- ResultMode: many
SELECT `Uuid`, `LastUpdate`
FROM `alpha`
WHERE `Animal` = 'cat' AND `LastUpdate` > '2024-01-01 00:00:00.000000'