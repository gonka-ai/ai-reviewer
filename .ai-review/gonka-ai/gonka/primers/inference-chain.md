---
id: inference-chain
type: overview
path_filters: ["./inference-chain/**"]
---
# Gonka Primary Chain
These files are part of the primary blockchain logic for Gonka. They use the Cosmos SDK to implement a level 1 blockchain.

Gonka is a live blockchain that orchestrates a decentralized AI inference network. 

Weight, rewards and voting power are all determined by periodic "Proof of Compute" (PoC) events, where each Participant runs transforms against a model to prove how much compute they have brought to the network.

Each Participant is also running an API server that servers the actual AI inferences and orchestrates PoCs (this is in the ./decentralized-api folder)

Inferences are registered on the blockchain (StartInference and FinishInference). They are randomly validated by other participants to make sure they have used the correct model.
