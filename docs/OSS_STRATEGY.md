# Open-source strategy

QH8Z uses permissive open-source software to accelerate commodity infrastructure while keeping the commercial product layer coherent and independently valuable.

## Shlink

Shlink is an included runtime dependency and the initial redirect/visit engine. It remains a distinct third-party component.

## Kutt

Kutt is currently a product/implementation reference rather than a bundled runtime. If substantial Kutt source is copied or adapted later, affected code must retain provenance and the Kutt MIT notice must continue to ship.

## QH8Z-owned code

The Node application, product schema, branded frontend, billing integration, abuse layer, and deployment topology are QH8Z-specific work unless a file says otherwise.

## Due diligence discipline

For every future code import: record upstream repo/tag/path/license, copy only what is useful, keep notices current, preserve upstream copyright notices, and review any non-permissive license before it enters the product.
