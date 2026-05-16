DELETE FROM rules
WHERE target_ids = '{}'::text[]
  AND enabled = FALSE
  AND name IN (
    'Requests: approval decisions',
    'Audiobooks: request status',
    'Ebooks: BookWarehouse request status',
    'Ebooks: EbookDB request status'
  );
