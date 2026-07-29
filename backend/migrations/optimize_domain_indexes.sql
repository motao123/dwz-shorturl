-- optimize_domain_indexes.sql
-- DWZ-H-02: indexes matching the actual domain picker and filtered list queries.
-- Run once on installations that already applied add_domains.sql.

ALTER TABLE `domains`
  ADD KEY `idx_pick_domain` (`status`, `link_count`, `priority`, `id`);

ALTER TABLE `short_urls`
  ADD KEY `idx_domain_created` (`domain_id`, `created_at`);
