-- Name: GetAllAnimals
SELECT `Animal`, `BigNumber`
FROM `alpha`;

-- Name: GetRecentCats
SELECT `Uuid`, `LastUpdate`
FROM `alpha`
WHERE `Animal` = 'cat' AND `LastUpdate` > '2024-01-01 00:00:00.000000';

-- Name: GetByUuid
SELECT `Animal`, `test_field`
FROM `alpha`
WHERE `Uuid` = ?;

-- Name: CountNullBigNumbers
SELECT COUNT(*) AS `count`
FROM `alpha`
WHERE `BigNumber` IS NULL;

-- Name: InsertOne
INSERT INTO `alpha` (`Uuid`, `Animal`, `test_field`)
VALUES (?, ?, ?);

-- Name: InsertHardcoded
INSERT INTO `alpha` (`Uuid`, `Animal`)
VALUES ('11111111-1111-4111-8111-111111111111', 'dog');

-- Name: UpdateAnimalName
UPDATE `alpha`
SET `Animal` = ?
WHERE `Uuid` = ?;

-- Name: UpdateTestField
UPDATE `alpha`
SET `test_field` = 'updated'
WHERE `Animal` = 'fox';

-- Name: DeleteByUuid
DELETE FROM `alpha` WHERE `Uuid` = ?;

-- Name: DeleteOldRows
DELETE FROM `alpha` WHERE `LastUpdate` < '2023-01-01 00:00:00.000000';
