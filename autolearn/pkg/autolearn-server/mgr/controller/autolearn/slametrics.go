package autolearn

import (
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/config"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/autolearn/pkg/autolearn-server/mgr/dao"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/es"
	"go.megvii-inc.com/brain/brainpp/projects/aiservice/pkg/mgolib"
)

func SyncToES(c *mgolib.MgoClient) {
	es.NewMongo2ESSyncer(
		es.AutoLearnRevisionIndexName,
		IndexTemplate,
		func() ([]string, error) {
			return []string{es.Tenant}, nil
		},
		func(tenant string) ([]any, error) {
			return dao.GetAllRevisionForES(dao.NewDAO(c, tenant))
		},
		config.Profile.ESConfig,
		nil,
	).Start()
}

func SyncTransformerToES(c *mgolib.MgoClient) {
	es.NewMongo2ESSyncer(
		es.AutoLearnTransformerRevisionIndexName,
		IndexTemplate,
		func() ([]string, error) {
			return []string{es.TransformerTenant}, nil
		},
		func(tenant string) ([]any, error) {
			return dao.GetAllRevisionForES(dao.NewDAO(c, tenant))
		},
		config.Profile.ESConfig,
		nil,
	).Start()
}

const IndexTemplate = `{
  "settings" : {
    "number_of_shards" : 1,
    "number_of_replicas" : 1,
	  "index.write.wait_for_active_shards": "all"
  },
  "mappings" : {
    "dynamic_templates" : [
      {
        "message_field" : {
          "path_match" : "message",
          "mapping" : {
            "norms" : false,
            "type" : "text"
          },
          "match_mapping_type" : "string"
        }
      },
      {
        "string_fields" : {
          "mapping" : {
            "norms" : false,
            "type" : "text",
            "fields" : {
              "keyword" : {
                "ignore_above" : 256,
                "type" : "keyword"
              }
            }
          },
          "match_mapping_type" : "string",
          "match" : "*"
        }
      }
    ],
    "properties" : {
      "@timestamp" : {
        "type" : "date_nanos"
      },
      "createdAt" : {
        "type" : "date",
		"format" : "epoch_second||yyyy/MM/dd HH:mm:ss||yyyy/MM/dd||epoch_millis"
      },
      "revisionCreatedAt" : {
        "type" : "date",
		"format" : "epoch_second||yyyy/MM/dd HH:mm:ss||yyyy/MM/dd||epoch_millis"
      }
    }
  }
}`
