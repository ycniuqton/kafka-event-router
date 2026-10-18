package kafka

import (
	"context"
	"errors"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
)

// GroupState is the broker-reported state of a consumer group (e.g. Empty, Stable, Dead).
type GroupState string

const (
	GroupEmpty  GroupState = "Empty"
	GroupStable GroupState = "Stable"
	GroupDead   GroupState = "Dead"
)

// GroupMember is one member of a consumer group and the partitions assigned to it.
type GroupMember struct {
	MemberID   string             `json:"memberId"`
	ClientID   string             `json:"clientId"`
	ClientHost string             `json:"clientHost"`
	Assigned   map[string][]int32 `json:"assigned"`
}

// GroupInfo describes a consumer group (replay waits until it is empty before resetting offsets).
type GroupInfo struct {
	Group   string        `json:"group"`
	State   GroupState    `json:"state"`
	Members []GroupMember `json:"members"`
}

// Empty reports whether no member is connected (offsets may be committed from outside).
func (g GroupInfo) Empty() bool { return len(g.Members) == 0 }

// PartitionOffsets maps topic → partition → offset.
type PartitionOffsets map[string]map[int32]int64

// Set stores one partition offset.
func (o PartitionOffsets) Set(topic string, partition int32, offset int64) {
	if o[topic] == nil {
		o[topic] = map[int32]int64{}
	}
	o[topic][partition] = offset
}

// DescribeGroup returns the state and members of one consumer group (an unknown group is Dead with no members).
func (a *FranzAdmin) DescribeGroup(ctx context.Context, group string) (GroupInfo, error) {
	described, err := a.adm.DescribeGroups(ctx, group)
	if err != nil {
		return GroupInfo{}, err
	}
	d, ok := described[group]
	if !ok || errors.Is(d.Err, kerr.GroupIDNotFound) {
		return GroupInfo{Group: group, State: GroupDead}, nil
	}
	if d.Err != nil {
		return GroupInfo{}, d.Err
	}
	info := GroupInfo{Group: group, State: GroupState(d.State), Members: make([]GroupMember, 0, len(d.Members))}
	for _, m := range d.Members {
		info.Members = append(info.Members, GroupMemberOf(m))
	}
	return info, nil
}

// GroupMemberOf converts a kadm member, decoding its consumer assignment when present.
func GroupMemberOf(m kadm.DescribedGroupMember) GroupMember {
	out := GroupMember{MemberID: m.MemberID, ClientID: m.ClientID, ClientHost: m.ClientHost, Assigned: map[string][]int32{}}
	if c, ok := m.Assigned.AsConsumer(); ok {
		for _, t := range c.Topics {
			out.Assigned[t.Topic] = t.Partitions
		}
	}
	return out
}

// OffsetsForTimes returns, per partition, the first offset whose timestamp is at or after at
// (the end offset when no such record exists).
func (a *FranzAdmin) OffsetsForTimes(ctx context.Context, at time.Time, topics ...string) (PartitionOffsets, error) {
	listed, err := a.adm.ListOffsetsAfterMilli(ctx, at.UnixMilli(), topics...)
	if err != nil {
		return nil, err
	}
	if err := listed.Error(); err != nil {
		return nil, err
	}
	out := PartitionOffsets{}
	listed.Each(func(o kadm.ListedOffset) { out.Set(o.Topic, o.Partition, o.Offset) })
	return out, nil
}

// CommitOffsets commits the offsets for group; it fails if any partition is rejected
// (e.g. the group still has active members).
func (a *FranzAdmin) CommitOffsets(ctx context.Context, group string, offsets PartitionOffsets) error {
	commit := kadm.Offsets{}
	for topic, partitions := range offsets {
		for partition, offset := range partitions {
			commit.Add(kadm.Offset{Topic: topic, Partition: partition, At: offset, LeaderEpoch: -1})
		}
	}
	return a.adm.CommitAllOffsets(ctx, group, commit)
}
