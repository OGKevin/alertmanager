-- name: InsertAlert :exec
insert into alerts (id, fingerprint, alertname) values ($1, $2, $3) on conflict do nothing;
