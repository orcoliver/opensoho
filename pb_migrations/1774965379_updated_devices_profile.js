/// <reference path="../pb_data/types.d.ts" />
migrate((app) => {
  const collection = app.findCollectionByNameOrId("pbc_2153001328")

  collection.fields.addAt(99, new Field({
    "cascadeDelete": false,
    "collectionId": "pbc_device_profile",
    "hidden": false,
    "id": "relation_dp_profile",
    "maxSelect": 1,
    "minSelect": 0,
    "name": "profile",
    "presentable": false,
    "required": false,
    "system": false,
    "type": "relation"
  }))

  return app.save(collection)
}, (app) => {
  const collection = app.findCollectionByNameOrId("pbc_2153001328")
  collection.fields.removeById("relation_dp_profile")
  return app.save(collection)
})
