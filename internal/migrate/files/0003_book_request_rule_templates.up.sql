INSERT INTO rules (name, event_pattern, target_ids, enabled, title, body)
VALUES
  (
    'Requests: approval decisions',
    'plugin.continuum.requests.*',
    '{}'::text[],
    FALSE,
    'Request event',
    '{{summary}}'
  ),
  (
    'Audiobooks: request status',
    'plugin.continuum.bookwarehouse-audio.request_*',
    '{}'::text[],
    FALSE,
    'Audiobook request update',
    '{{summary}}'
  ),
  (
    'Ebooks: BookWarehouse request status',
    'plugin.continuum.bookwarehouse-ebook.request_*',
    '{}'::text[],
    FALSE,
    'Ebook request update',
    '{{summary}}'
  ),
  (
    'Ebooks: EbookDB request status',
    'plugin.continuum.ebookdb.request_*',
    '{}'::text[],
    FALSE,
    'Ebook request update',
    '{{summary}}'
  )
ON CONFLICT DO NOTHING;
