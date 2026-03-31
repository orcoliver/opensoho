/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  const collection = app.findCollectionByNameOrId("pbc_502121861")

  collection.fields.addAt(20, new Field({
    "hidden": false,
    "id": "bool_ft_over_ds",
    "name": "ft_over_ds",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "bool"
  }))

  collection.fields.addAt(21, new Field({
    "hidden": false,
    "id": "bool_ft_psk_generate_local",
    "name": "ft_psk_generate_local",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "bool"
  }))

  collection.fields.addAt(22, new Field({
    "autogeneratePattern": "",
    "hidden": false,
    "id": "text_mobility_domain",
    "max": 4,
    "min": 0,
    "name": "mobility_domain",
    "pattern": "^[0-9a-fA-F]{4}$|^$",
    "presentable": false,
    "primaryKey": false,
    "required": false,
    "system": false,
    "type": "text"
  }))

  collection.fields.addAt(23, new Field({
    "autogeneratePattern": "",
    "hidden": false,
    "id": "text_nasid",
    "max": 48,
    "min": 0,
    "name": "nasid",
    "pattern": "",
    "presentable": false,
    "primaryKey": false,
    "required": false,
    "system": false,
    "type": "text"
  }))

  collection.fields.addAt(24, new Field({
    "hidden": false,
    "id": "bool_rrm_neighbor_report",
    "name": "rrm_neighbor_report",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "bool"
  }))

  collection.fields.addAt(25, new Field({
    "hidden": false,
    "id": "bool_rrm_beacon_report",
    "name": "rrm_beacon_report",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "bool"
  }))

  return app.save(collection)
}, (app) => {
  const collection = app.findCollectionByNameOrId("pbc_502121861")

  collection.fields.removeById("bool_ft_over_ds")
  collection.fields.removeById("bool_ft_psk_generate_local")
  collection.fields.removeById("text_mobility_domain")
  collection.fields.removeById("text_nasid")
  collection.fields.removeById("bool_rrm_neighbor_report")
  collection.fields.removeById("bool_rrm_beacon_report")

  return app.save(collection)
})
