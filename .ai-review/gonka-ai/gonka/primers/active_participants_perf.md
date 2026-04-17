---
id: active_participants_perf
path_filters: [ "./inference-chain/**/*.go" ]
regex_filters: [ "ActiveParticipants" ]
---
ActiveParticipants is expensive to load, ActiveParticipantsSet is optimized for checking ActiveParticipant membership in a specific Epoch for a specific Participant. 

