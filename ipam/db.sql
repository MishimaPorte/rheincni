create table if not exists ip_allocation (
    ip           integer not null primary key,
    veth_index   integer not null,
    owner_type   text not null default 'pod' check (owner_type in ('pod', 'router')),
    container_id text null default null,
    alloc_ts     timestamp not null default current_timestamp
);
