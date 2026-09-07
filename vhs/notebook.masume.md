+++
title = "revenue review"
profiles = ["shop"]
engine = "postgres"

[run]
transaction = "autocommit"
on_error = "stop"
+++

# Revenue review

Revenue and orders of the shop. Every statement cell binds :status.

```param id=values
status = 'refunded'
```

```sql id=countries-by-revenue
-- countries by revenue
select c.country,
       count(*) as orders,
       round(sum(o.total), 2) as revenue
from orders o join customers c on c.id = o.customer_id
where o.status <> :status
group by c.country
order by revenue desc
```

```chart id=bars source=countries-by-revenue label=country value=revenue kind=bar sort=desc
```

```sql id=orders-per-status
-- orders per status
select status, count(*) as orders, round(sum(total), 2) as revenue
from orders group by status order by orders desc
```
