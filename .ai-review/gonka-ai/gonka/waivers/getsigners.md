---
id: getsigners
model_category: balanced
regex_filters: [ "GetSigners" ]
path_filters: [ "inference-chain/**/*.go" ]
exclude_filters: [ "**/*_test.go", "**/*.pb.go", "**/*.pulsar.go", "inference-chain/testutil/**" ]
---
The GetSigners method in Cosmos SDK was deprecated. Instead, it uses a protobuf option as follows: 
```protobuf
message MsgCreateGame {
  option (cosmos.msg.v1.signer) = "creator";

  // creator is the message sender.
  string creator = 1;
  string index = 2 ;
  string black = 3 [(cosmos_proto.scalar) = "cosmos.AddressString"];
  string red = 4 [(cosmos_proto.scalar) = "cosmos.AddressString"];
}

```

There is no longer a need to directly implement the GetSigners method, and other implementations will not cause a problem