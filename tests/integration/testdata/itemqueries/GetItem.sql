-- Params: id
-- Returns: id label detail
-- ResultMode: one
-- MapAs: embedded_items
SELECT id, label, detail FROM embedded_items WHERE id = ?;
