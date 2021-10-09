package minidyn

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbiface"
	"github.com/stretchr/testify/require"
	"github.com/truora/minidyn/interpreter"
)

const tableName = "pokemons"

type pokemon struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	SecondType string `json:"second_type"`
	Name       string `json:"name"`
}

func ensurePokemonTable(client dynamodbiface.DynamoDBAPI) error {
	err := AddTable(client, tableName, "id", "")

	var aerr awserr.Error
	ok := errors.As(err, &aerr)

	if !ok || aerr.Code() != dynamodb.ErrCodeResourceInUseException {
		return err
	}

	return nil
}

func ensurePokemonTypeIndex(client dynamodbiface.DynamoDBAPI) error {
	err := AddIndex(client, tableName, "by-type", "type", "id")

	var aerr awserr.Error
	ok := errors.As(err, &aerr)

	if !ok || aerr.Code() != "ValidationException" {
		return err
	}

	return nil
}

func createPokemon(client dynamodbiface.DynamoDBAPI, creature pokemon) error {
	item, err := dynamodbattribute.MarshalMap(creature)
	if err != nil {
		return err
	}

	input := &dynamodb.PutItemInput{
		Item:      item,
		TableName: aws.String(tableName),
	}

	_, err = client.PutItemWithContext(context.Background(), input)

	return err
}

func getPokemon(client dynamodbiface.DynamoDBAPI, id string) (map[string]*dynamodb.AttributeValue, error) {
	key := map[string]*dynamodb.AttributeValue{
		"id": {
			S: aws.String(id),
		},
	}

	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       key,
	}

	out, err := client.GetItemWithContext(context.Background(), getInput)

	return out.Item, err
}

func getPokemonsByType(client dynamodbiface.DynamoDBAPI, typ string) ([]map[string]*dynamodb.AttributeValue, error) {
	input := &dynamodb.QueryInput{
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":type": {
				S: aws.String(typ),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#type": aws.String("type"),
		},
		KeyConditionExpression: aws.String("#type = :type"),
		TableName:              aws.String(tableName),
		IndexName:              aws.String("by-type"),
	}

	items := []map[string]*dynamodb.AttributeValue{}

	out, err := client.QueryWithContext(context.Background(), input)
	if err == nil {
		items = out.Items
	}

	return items, err
}

func setupClient(table string) dynamodbiface.DynamoDBAPI {
	dynamodbEndpoint := os.Getenv("LOCAL_DYNAMODB_ENDPOINT")
	if dynamodbEndpoint != "" {
		config := &aws.Config{}
		config.Endpoint = aws.String(dynamodbEndpoint)

		return dynamodb.New(session.Must(session.NewSession(config)))
	}

	client := NewClient()
	setupNativeInterpreter(client.GetNativeInterpreter(), table)

	return client
}

func setupNativeInterpreter(native *interpreter.Native, table string) {
	native.AddUpdater(table, "SET second_type = :ntype", func(item map[string]*dynamodb.AttributeValue, updates map[string]*dynamodb.AttributeValue) {
		item["second_type"] = updates[":ntype"]
	})

	native.AddUpdater(table, "SET #type = :ntype", func(item map[string]*dynamodb.AttributeValue, updates map[string]*dynamodb.AttributeValue) {
		item["type"] = updates[":ntype"]
	})
}

func getData(client dynamodbiface.DynamoDBAPI, tn, p, r string) (map[string]*dynamodb.AttributeValue, error) {
	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(tn),
		Key: map[string]*dynamodb.AttributeValue{
			"partition": {
				S: aws.String(p),
			},
			"range": {
				S: aws.String(r),
			},
		},
	}

	out, err := client.GetItemWithContext(context.Background(), getInput)

	return out.Item, err
}

func getDataInIndex(client dynamodbiface.DynamoDBAPI, index, tn, p, r string) ([]map[string]*dynamodb.AttributeValue, error) {
	input := &dynamodb.QueryInput{
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":data": {
				S: aws.String(p),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#data": aws.String("data"),
		},
		KeyConditionExpression: aws.String("#data = :data"),
		TableName:              aws.String(tn),
		IndexName:              aws.String(index),
	}

	items := []map[string]*dynamodb.AttributeValue{}

	out, err := client.QueryWithContext(context.Background(), input)
	if err == nil {
		items = out.Items
	}

	return items, err
}

func TestSetInterpreter(t *testing.T) {
	c := require.New(t)
	client := NewClient()

	c.NotPanics(func() {
		_, err := client.getTable("tests")
		c.Error(err)
	})

	_ = AddTable(client, "tests", "hk", "rk")

	native := interpreter.NewNativeInterpreter()
	client.SetInterpreter(native)

	_, err := client.getTable("tests")
	c.NoError(err)
}

func TestActivateDebug(t *testing.T) {
	c := require.New(t)
	fake := NewClient()

	fake.ActivateDebug()

	c.True(fake.langInterpreter.Debug)
}

func TestCreateTable(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	input := &dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("partition"),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("partition"),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String("range"),
				KeyType:       aws.String("RANGE"),
			},
		},
		TableName: aws.String(tableName),
	}

	_, err := client.CreateTableWithContext(context.Background(), input)
	c.Contains(err.Error(), "Range Key not specified in Attribute Definitions")

	input.AttributeDefinitions = append(input.AttributeDefinitions, &dynamodb.AttributeDefinition{
		AttributeName: aws.String("range"),
		AttributeType: aws.String("S"),
	})

	_, err = client.CreateTableWithContext(context.Background(), input)
	c.Contains(err.Error(), "No provisioned throughput specified for the table")

	input.BillingMode = aws.String("PAY_PER_REQUEST")

	_, err = client.CreateTableWithContext(context.Background(), input)
	c.NoError(err)

	_, err = client.CreateTableWithContext(context.Background(), input)
	c.Contains(err.Error(), "Cannot create preexisting table")
}

func TestCreateTableWithGSI(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	input := &dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("partition"),
				AttributeType: aws.String("S"),
			},
			{
				AttributeName: aws.String("range"),
				AttributeType: aws.String("S"),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("partition"),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String("range"),
				KeyType:       aws.String("RANGE"),
			},
		},
		GlobalSecondaryIndexes: []*dynamodb.GlobalSecondaryIndex{},
		TableName:              aws.String(tableName + "-gsi"),
	}

	_, err := client.CreateTableWithContext(context.Background(), input)
	c.Contains(err.Error(), "GSI list is empty/invalid")

	input.GlobalSecondaryIndexes = append(input.GlobalSecondaryIndexes, &dynamodb.GlobalSecondaryIndex{
		IndexName: aws.String("invert"),
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("range"),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String("no_defined"),
				KeyType:       aws.String("RANGE"),
			},
		},
		Projection: &dynamodb.Projection{
			ProjectionType: aws.String("ALL"),
		},
	})

	_, err = client.CreateTableWithContext(context.Background(), input)
	c.Contains(err.Error(), "No provisioned throughput specified for the global secondary index")

	input.GlobalSecondaryIndexes[0].ProvisionedThroughput = &dynamodb.ProvisionedThroughput{
		ReadCapacityUnits:  aws.Int64(1),
		WriteCapacityUnits: aws.Int64(1),
	}

	_, err = client.CreateTableWithContext(context.Background(), input)
	c.Contains(err.Error(), "Global Secondary Index range key not specified in Attribute")

	input.GlobalSecondaryIndexes[0].KeySchema[1].AttributeName = aws.String("partition")

	_, err = client.CreateTableWithContext(context.Background(), input)
	c.NoError(err)
}

func TestCreateTableWithLSI(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	input := &dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("partition"),
				AttributeType: aws.String("S"),
			},
			{
				AttributeName: aws.String("range"),
				AttributeType: aws.String("S"),
			},
			{
				AttributeName: aws.String("data"),
				AttributeType: aws.String("S"),
			},
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(1),
			WriteCapacityUnits: aws.Int64(1),
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("partition"),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String("range"),
				KeyType:       aws.String("RANGE"),
			},
		},
		LocalSecondaryIndexes: []*dynamodb.LocalSecondaryIndex{},
		TableName:             aws.String(tableName + "-lsi"),
	}

	_, err := client.CreateTableWithContext(context.Background(), input)
	c.Contains(err.Error(), "LSI list is empty/invalid")

	input.LocalSecondaryIndexes = append(input.LocalSecondaryIndexes, &dynamodb.LocalSecondaryIndex{
		IndexName: aws.String("data"),
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("partition"),
				KeyType:       aws.String("HASH"),
			},
			{
				AttributeName: aws.String("no_defined"),
				KeyType:       aws.String("RANGE"),
			},
		},
		Projection: &dynamodb.Projection{
			ProjectionType: aws.String("ALL"),
		},
	})

	_, err = client.CreateTableWithContext(context.Background(), input)
	c.Contains(err.Error(), "Local Secondary Index range key not specified in Attribute")

	input.LocalSecondaryIndexes[0].KeySchema[1].AttributeName = aws.String("data")

	_, err = client.CreateTableWithContext(context.Background(), input)
	c.NoError(err)
}

func TestDeleteTable(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	input := &dynamodb.DeleteTableInput{
		TableName: aws.String("table-404"),
	}

	_, err := client.DeleteTableWithContext(context.Background(), input)
	c.Equal("ResourceNotFoundException: Cannot do operations on a non-existent table", err.Error())

	err = ensurePokemonTable(client)
	c.NoError(err)

	input = &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	}
	out, err := client.DeleteTableWithContext(context.Background(), input)
	c.NoError(err)

	c.NotEmpty(out)

	_, err = client.DeleteTableWithContext(context.Background(), input)
	c.Equal("ResourceNotFoundException: Cannot do operations on a non-existent table", err.Error())
}

func TestUpdateTable(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	input := &dynamodb.UpdateTableInput{
		BillingMode:                 aws.String("PROVISIONED"),
		GlobalSecondaryIndexUpdates: []*dynamodb.GlobalSecondaryIndexUpdate{},
		TableName:                   aws.String("404"),
	}

	_, err := client.UpdateTableWithContext(context.Background(), input)
	c.Equal("ResourceNotFoundException: Cannot do operations on a non-existent table", err.Error())

	err = ensurePokemonTable(client)
	c.NoError(err)

	input = &dynamodb.UpdateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("id"),
				AttributeType: aws.String("S"),
			},
			{
				AttributeName: aws.String("type"),
				AttributeType: aws.String("S"),
			},
		},
		GlobalSecondaryIndexUpdates: []*dynamodb.GlobalSecondaryIndexUpdate{
			{
				Create: &dynamodb.CreateGlobalSecondaryIndexAction{
					IndexName: aws.String("newIndex"),
					KeySchema: []*dynamodb.KeySchemaElement{
						{
							AttributeName: aws.String("type"),
							KeyType:       aws.String("HASH"),
						},
						{
							AttributeName: aws.String("id"),
							KeyType:       aws.String("RANGE"),
						},
					},
					Projection: &dynamodb.Projection{
						ProjectionType: aws.String("ALL"),
					},
				},
			},
		},
		TableName: aws.String(tableName),
	}
	output, err := client.UpdateTableWithContext(context.Background(), input)
	c.NoError(err)

	c.Len(output.TableDescription.GlobalSecondaryIndexes, 1)

	input = &dynamodb.UpdateTableInput{
		GlobalSecondaryIndexUpdates: []*dynamodb.GlobalSecondaryIndexUpdate{
			{
				Update: &dynamodb.UpdateGlobalSecondaryIndexAction{
					IndexName: aws.String("newIndex"),
					ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
						ReadCapacityUnits:  aws.Int64(1),
						WriteCapacityUnits: aws.Int64(1),
					},
				},
			},
		},
		TableName: aws.String(tableName),
	}
	_, err = client.UpdateTableWithContext(context.Background(), input)
	c.NoError(err)

	input = &dynamodb.UpdateTableInput{
		GlobalSecondaryIndexUpdates: []*dynamodb.GlobalSecondaryIndexUpdate{
			{
				Delete: &dynamodb.DeleteGlobalSecondaryIndexAction{
					IndexName: aws.String("newIndex"),
				},
			},
		},
		TableName: aws.String(tableName),
	}
	_, err = client.UpdateTableWithContext(context.Background(), input)
	c.NoError(err)

	_, err = client.UpdateTableWithContext(context.Background(), input)
	c.Equal("ResourceNotFoundException: Requested resource not found", err.Error())
}

func TestPutAndGetItem(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	item, err := dynamodbattribute.MarshalMap(pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	input := &dynamodb.PutItemInput{
		Item:      item,
		TableName: aws.String(tableName),
	}

	_, err = client.PutItemWithContext(context.Background(), input)
	c.NoError(err)

	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String("001"),
			},
		},
	}
	out, err := client.GetItemWithContext(context.Background(), getInput)
	c.NoError(err)
	c.Equal("001", *out.Item["id"].S)
	c.Equal("Bulbasaur", *out.Item["name"].S)
	c.Equal("grass", *out.Item["type"].S)

	_, err = client.GetItemWithContext(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key:       map[string]*dynamodb.AttributeValue{},
	})

	c.Contains(err.Error(), ErrMissingKeys.Error())
}

func TestPutItemWithConditions(t *testing.T) {
	c := require.New(t)

	client := setupClient(tableName)
	err := ensurePokemonTable(client)
	c.NoError(err)

	item, err := dynamodbattribute.MarshalMap(pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	input := &dynamodb.PutItemInput{
		Item:                item,
		TableName:           aws.String(tableName),
		ConditionExpression: aws.String("attribute_not_exists(#type)"),
		ExpressionAttributeNames: map[string]*string{
			"#type": aws.String("type"),
		},
	}

	_, err = client.PutItemWithContext(context.Background(), input)
	c.NoError(err)

	_, err = client.PutItemWithContext(context.Background(), input)
	c.Error(err)
}

func TestUpdateItemWithContext(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	expr := map[string]*dynamodb.AttributeValue{
		":ntype": {
			S: aws.String(string("poison")),
		},
	}
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String("001"),
			},
		},
		ReturnValues:              aws.String("UPDATED_NEW"),
		UpdateExpression:          aws.String("SET second_type = :ntype"),
		ExpressionAttributeValues: expr,
	}

	_, err = client.UpdateItemWithContext(context.Background(), input)
	c.NoError(err)

	item, err := getPokemon(client, "001")
	c.NoError(err)
	c.Equal("poison", aws.StringValue(item["second_type"].S))

	input.Key["id"] = &dynamodb.AttributeValue{
		S: aws.String("404"),
	}

	_, err = client.UpdateItemWithContext(context.Background(), input)
	c.NoError(err)

	item, err = getPokemon(client, "404")
	c.NoError(err)
	c.Equal("poison", aws.StringValue(item["second_type"].S))
}

func TestUpdateItemWithConditionalExpression(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "002",
		Type: "grass",
		Name: "Ivysaur",
	})
	c.NoError(err)

	uexpr := "SET second_type = :ntype"
	expr := map[string]*dynamodb.AttributeValue{
		":ntyp": {
			S: aws.String(string("poison")),
		},
	}
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String("404"),
			},
		},
		ConditionExpression:       aws.String("attribute_exists(id)"),
		ReturnValues:              aws.String("UPDATED_NEW"),
		UpdateExpression:          aws.String(uexpr),
		ExpressionAttributeValues: expr,
		ExpressionAttributeNames: map[string]*string{
			"#id": aws.String("id"),
		},
	}

	_, err = client.UpdateItemWithContext(context.Background(), input)
	c.Contains(err.Error(), dynamodb.ErrCodeConditionalCheckFailedException)
}

func TestUpdateItemWithGSI(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = ensurePokemonTypeIndex(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{ID: "001", Type: "grass", Name: "Bulbasaur"})
	c.NoError(err)

	items, err := getPokemonsByType(client, "grass")
	c.NoError(err)
	c.Len(items, 1)

	uexpr := "SET #type = :ntype"
	expr := map[string]*dynamodb.AttributeValue{
		":ntype": {
			S: aws.String(string("poison")),
		},
	}
	names := map[string]*string{"#type": aws.String("type")}

	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String("001"),
			},
		},
		ReturnValues:              aws.String("UPDATED_NEW"),
		UpdateExpression:          aws.String(uexpr),
		ExpressionAttributeValues: expr,
		ExpressionAttributeNames:  names,
	}

	_, err = client.UpdateItem(input)
	c.NoError(err)

	items, err = getPokemonsByType(client, "grass")
	c.NoError(err)
	c.Empty(items)

	items, err = getPokemonsByType(client, "poison")
	c.NoError(err)
	c.Len(items, 1)
}

func TestUpdateItemError(t *testing.T) {
	c := require.New(t)

	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = ensurePokemonTypeIndex(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{ID: "001", Type: "grass", Name: "Bulbasaur"})
	c.NoError(err)

	expr := map[string]*dynamodb.AttributeValue{
		":second_type": {
			S: aws.String(string("poison")),
		},
	}
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(tableName),
		Key: map[string]*dynamodb.AttributeValue{
			"foo": {
				S: aws.String("a"),
			},
		},
		ReturnValues:              aws.String("UPDATED_NEW"),
		UpdateExpression:          aws.String("SET second_type = :second_type"),
		ExpressionAttributeValues: expr,
	}

	_, err = client.UpdateItem(input)
	c.Contains(err.Error(), ErrMissingKeys.Error())

	ActiveForceFailure(client)
	defer DeactiveForceFailure(client)

	output, err := client.UpdateItemWithContext(context.Background(), input)
	c.EqualError(err, "forced failure response")
	c.Nil(output)
}

func TestQueryWithContext(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "004",
		Type: "fire",
		Name: "Charmander",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "007",
		Type: "water",
		Name: "Squirtle",
	})
	c.NoError(err)

	input := &dynamodb.QueryInput{
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id": {
				S: aws.String("004"),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#id": aws.String("id"),
		},
		KeyConditionExpression: aws.String("#id = :id"),
		TableName:              aws.String(tableName),
	}

	out, err := client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 1)
	c.Empty(out.LastEvaluatedKey)

	input.FilterExpression = aws.String("#type = :type")

	input.ExpressionAttributeNames["#type"] = aws.String("type")
	input.ExpressionAttributeValues[":type"] = &dynamodb.AttributeValue{
		S: aws.String("fire"),
	}

	out, err = client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 1)

	input.ExpressionAttributeValues[":type"] = &dynamodb.AttributeValue{
		S: aws.String("grass"),
	}

	out, err = client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Empty(out.Items)
}

func TestQueryWithContextPagination(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = ensurePokemonTypeIndex(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "002",
		Type: "grass",
		Name: "Ivysaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "003",
		Type: "grass",
		Name: "Venusaur",
	})
	c.NoError(err)

	input := &dynamodb.QueryInput{
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":type": {
				S: aws.String("grass"),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#type": aws.String("type"),
		},
		KeyConditionExpression: aws.String("#type = :type"),
		TableName:              aws.String(tableName),
		IndexName:              aws.String("by-type"),
		Limit:                  aws.Int64(1),
	}

	out, err := client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 1)
	c.Equal("001", aws.StringValue(out.Items[0]["id"].S))

	input.ExclusiveStartKey = out.LastEvaluatedKey
	out, err = client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 1)
	c.Equal("002", aws.StringValue(out.Items[0]["id"].S))

	input.ExclusiveStartKey = out.LastEvaluatedKey
	out, err = client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 1)
	c.Equal("003", aws.StringValue(out.Items[0]["id"].S))

	input.ExclusiveStartKey = out.LastEvaluatedKey
	out, err = client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Empty(out.Items)
	c.Empty(out.LastEvaluatedKey)

	err = createPokemon(client, pokemon{
		ID:   "004",
		Type: "fire",
		Name: "Charmander",
	})
	c.NoError(err)

	input.ExclusiveStartKey = nil
	input.Limit = aws.Int64(4)

	out, err = client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 3)
	c.Empty(out.LastEvaluatedKey)

	// Query with FilterExpression
	input.ExclusiveStartKey = nil
	input.FilterExpression = aws.String("begins_with(#name, :letter)")
	input.Limit = aws.Int64(2)
	input.ExpressionAttributeValues[":letter"] = &dynamodb.AttributeValue{
		S: aws.String("V"),
	}
	input.ExpressionAttributeNames["#name"] = aws.String("name")

	out, err = client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Empty(out.Items)
	c.NotEmpty(out.LastEvaluatedKey)

	input.ExclusiveStartKey = out.LastEvaluatedKey
	out, err = client.QueryWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 1)
	c.Equal("003", aws.StringValue(out.Items[0]["id"].S))
}

func TestQuerySyntaxError(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	input := &dynamodb.QueryInput{
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":partition": {
				S: aws.String("a"),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#partition": aws.String("partition"),
		},
		// Syntax Error
		KeyConditionExpression: aws.String("#partition != :partition"),
		TableName:              aws.String(tableName),
	}

	c.Panics(func() {
		_, _ = client.QueryWithContext(context.Background(), input)
	})
}

func TestScanWithContext(t *testing.T) {
	c := require.New(t)

	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "002",
		Type: "grass",
		Name: "Ivysaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "003",
		Type: "grass",
		Name: "Venusaur",
	})
	c.NoError(err)

	input := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
	}

	out, err := client.ScanWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 3)

	input.Limit = aws.Int64(1)
	out, err = client.ScanWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 1)
	c.NotEmpty(out.LastEvaluatedKey)

	input.Limit = nil
	input.FilterExpression = aws.String("#name = :name")
	input.ExpressionAttributeValues = map[string]*dynamodb.AttributeValue{
		":name": {
			S: aws.String("Venusaur"),
		},
	}
	input.ExpressionAttributeNames = map[string]*string{
		"#name": aws.String("name"),
	}

	out, err = client.ScanWithContext(context.Background(), input)
	c.NoError(err)
	c.Len(out.Items, 1)

	if fclient, isFake := client.(*Client); isFake {
		ActiveForceFailure(fclient)

		out, err = client.ScanWithContext(context.Background(), input)
		c.Equal(ErrForcedFailure, err)
		c.Empty(out)

		DeactiveForceFailure(fclient)
	}
}

func TestDeleteItemWithContext(t *testing.T) {
	c := require.New(t)

	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = ensurePokemonTypeIndex(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "002",
		Type: "grass",
		Name: "Ivysaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "003",
		Type: "grass",
		Name: "Venusaur",
	})
	c.NoError(err)

	key := map[string]*dynamodb.AttributeValue{
		"id": {S: aws.String("003")},
	}

	input := &dynamodb.DeleteItemInput{
		Key:       key,
		TableName: aws.String(tableName),
	}

	items, err := getPokemonsByType(client, "grass")
	c.NoError(err)
	c.Len(items, 3)

	_, err = client.DeleteItemWithContext(context.Background(), input)
	c.NoError(err)

	item, err := getPokemon(client, "003")
	c.NoError(err)

	c.Empty(item)

	items, err = getPokemonsByType(client, "grass")
	c.NoError(err)
	c.Len(items, 2)

	_, err = client.DeleteItemWithContext(context.Background(), input)
	c.NoError(err)

	if _, ok := client.(*Client); ok {
		EmulateFailure(client, FailureConditionInternalServerError)

		defer func() { EmulateFailure(client, FailureConditionNone) }()

		output, forcedError := client.DeleteItemWithContext(context.Background(), input)
		c.Nil(output)

		var aerr awserr.Error
		ok := errors.As(forcedError, &aerr)

		c.True(ok)
		c.Equal(dynamodb.ErrCodeInternalServerError, aerr.Code())
	}
}

func TestDeleteItemWithConditions(t *testing.T) {
	c := require.New(t)

	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	input := &dynamodb.DeleteItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"id": {S: aws.String("001")},
		},
		TableName:           aws.String(tableName),
		ConditionExpression: aws.String("attribute_exists(id)"),
	}

	_, err = client.DeleteItemWithContext(context.Background(), input)
	c.NoError(err)

	_, err = client.DeleteItemWithContext(context.Background(), input)
	c.Contains(err.Error(), dynamodb.ErrCodeConditionalCheckFailedException)
}

func TestDeleteItemWithContextWithReturnValues(t *testing.T) {
	c := require.New(t)

	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	input := &dynamodb.DeleteItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"id": {S: aws.String("001")},
		},
		TableName:    aws.String(tableName),
		ReturnValues: aws.String("ALL_OLD"),
	}

	output, err := client.DeleteItemWithContext(context.Background(), input)
	c.NoError(err)

	c.Equal("Bulbasaur", *output.Attributes["name"].S)
}

func TestDescribeTable(t *testing.T) {
	c := require.New(t)

	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	describeTableInput := &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}

	output, err := client.DescribeTableWithContext(aws.BackgroundContext(), describeTableInput)
	c.NoError(err)
	c.NotNil(output)
	c.Len(output.Table.KeySchema, 1)
	c.Equal(aws.StringValue(output.Table.TableName), tableName)
	c.Equal(aws.StringValue(output.Table.KeySchema[0].KeyType), "HASH")
	c.Equal(aws.StringValue(output.Table.KeySchema[0].AttributeName), "id")
}

func TestDescribeTableFail(t *testing.T) {
	c := require.New(t)
	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	expected := "ResourceNotFoundException: Cannot do operations on a non-existent table"
	describeTableInput := &dynamodb.DescribeTableInput{
		TableName: aws.String("non_existing"),
	}

	output, err := client.DescribeTableWithContext(aws.BackgroundContext(), describeTableInput)
	c.Error(err)
	c.Equal(expected, err.Error())
	c.Empty(output)
}

func TestBatchWriteItemWithContext(t *testing.T) {
	c := require.New(t)
	client := NewClient()

	err := ensurePokemonTable(client)
	c.NoError(err)

	m := pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	}

	item, err := dynamodbattribute.MarshalMap(m)
	c.NoError(err)

	requests := []*dynamodb.WriteRequest{
		{
			PutRequest: &dynamodb.PutRequest{
				Item: item,
			},
		},
	}

	input := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]*dynamodb.WriteRequest{
			tableName: requests,
		},
	}

	itemCollectionMetrics := map[string][]*dynamodb.ItemCollectionMetrics{
		"table": {
			{
				ItemCollectionKey: map[string]*dynamodb.AttributeValue{
					"report_id": {
						S: aws.String("1234"),
					},
				},
			},
		},
	}

	SetItemCollectionMetrics(client, itemCollectionMetrics)

	output, err := client.BatchWriteItemWithContext(context.Background(), input)
	c.NoError(err)
	c.NotEmpty(output.ItemCollectionMetrics)

	collectionKeys, ok := output.ItemCollectionMetrics["table"]
	c.True(ok)
	c.Equal(1, len(collectionKeys))

	key, ok := collectionKeys[0].ItemCollectionKey["report_id"]
	c.True(ok)
	c.NotEmpty(key.S)
	c.Equal("1234", *key.S)

	ReturnUnprocessedItemsInBatch = true

	defer func() { ReturnUnprocessedItemsInBatch = false }()

	output, err = client.BatchWriteItemWithContext(context.Background(), input)
	c.NoError(err)
	c.NotEmpty(output.UnprocessedItems)

	ActiveForceFailure(client)
	defer DeactiveForceFailure(client)

	_, err = client.BatchWriteItemWithContext(context.Background(), input)
	c.Equal(ErrForcedFailure, err)
}

func TestTransactWriteItemsWithContext(t *testing.T) {
	c := require.New(t)
	client := NewClient()

	err := ensurePokemonTable(client)
	c.NoError(err)

	transactItems := []*dynamodb.TransactWriteItem{
		{
			Update: &dynamodb.Update{
				Key: map[string]*dynamodb.AttributeValue{
					"id":     {S: aws.String("001")},
					":ntype": {S: aws.String("poison")},
				},
				TableName:        aws.String(tableName),
				UpdateExpression: aws.String("SET second_type = :ntype"),
				ExpressionAttributeNames: map[string]*string{
					"#id": aws.String("id"),
				},
				ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
					":update": {S: aws.String(time.Now().Format(time.RFC3339))},
					":incr": {
						N: aws.String("1"),
					},
					":initial": {
						N: aws.String("0"),
					},
				},
			},
		},
	}

	writeItemsInput := &dynamodb.TransactWriteItemsInput{
		TransactItems: transactItems,
	}

	output, err := client.TransactWriteItemsWithContext(context.Background(), writeItemsInput)
	c.NoError(err)
	c.NotNil(output)

	ActiveForceFailure(client)
	defer DeactiveForceFailure(client)

	_, err = client.TransactWriteItemsWithContext(context.Background(), writeItemsInput)
	c.Equal(ErrForcedFailure, err)
}

func TestCheckTableName(t *testing.T) {
	c := require.New(t)

	fclient := NewClient()
	native := fclient.GetNativeInterpreter()
	setupNativeInterpreter(native, tableName)

	_, err := fclient.getTable(tableName)
	expected := "ResourceNotFoundException: Cannot do operations on a non-existent table"
	c.Equal(expected, err.Error())

	err = AddTable(fclient, "new-table", "partition", "range")
	c.NoError(err)

	_, err = fclient.getTable("notATable")
	c.Equal(expected, err.Error())
}

func TestForceFailure(t *testing.T) {
	c := require.New(t)

	client := NewClient()
	native := client.GetNativeInterpreter()

	setupNativeInterpreter(native, tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	ActiveForceFailure(client)
	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.Equal(ErrForcedFailure, err)

	DeactiveForceFailure(client)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)
	c.NoError(err)

	actualClient := dynamodb.New(session.Must(session.NewSession(&aws.Config{})))

	c.Panics(func() {
		ActiveForceFailure(actualClient)
	})
	c.Panics(func() {
		DeactiveForceFailure(actualClient)
	})
	c.Panics(func() {
		SetItemCollectionMetrics(actualClient, nil)
	})
}

func BenchmarkQuery(b *testing.B) {
	c := require.New(b)
	client := setupClient(tableName)

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = ensurePokemonTypeIndex(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "002",
		Type: "grass",
		Name: "Ivysaur",
	})
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "003",
		Type: "grass",
		Name: "Venusaur",
	})
	c.NoError(err)

	input := &dynamodb.QueryInput{
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":type": {
				S: aws.String("grass"),
			},
		},
		ExpressionAttributeNames: map[string]*string{
			"#type": aws.String("type"),
		},
		KeyConditionExpression: aws.String("#type = :type"),
		TableName:              aws.String(tableName),
		IndexName:              aws.String("by-type"),
		Limit:                  aws.Int64(1),
	}

	for n := 0; n < b.N; n++ {
		out, err := client.QueryWithContext(context.Background(), input)
		c.NoError(err)
		c.Len(out.Items, 1)
	}
}

func BenchmarkQueryWithContext(b *testing.B) {
	c := require.New(b)
	client := NewClient()

	err := ensurePokemonTable(client)
	c.NoError(err)

	err = createPokemon(client, pokemon{
		ID:   "001",
		Type: "grass",
		Name: "Bulbasaur",
	})
	c.NoError(err)

	input := &dynamodb.QueryInput{
		ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
			":id": {
				S: aws.String("001"),
			},
		},
		KeyConditionExpression:   aws.String("#id = :id"),
		ExpressionAttributeNames: map[string]*string{"#id": aws.String("id")},
		TableName:                aws.String(tableName),
		Limit:                    aws.Int64(1),
	}

	ctx := context.Background()

	for i := 0; i < b.N; i++ {
		_, err := client.QueryWithContext(ctx, input)
		if err != nil {
			b.Error(err)
		}
	}
}
