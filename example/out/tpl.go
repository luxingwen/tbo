package data

type KV struct{
    Key             uint64 //  键
    Value           interface{} //  值
}

type User struct{
    Level           uint64 //  等级
    MaxEnergy       uint64 //  体力上限
    Perexp          uint64 //  单次钓鱼经验
    Exp             uint64 //  升级经验
    Reward          []*KV //  升级奖励
    Attrs           []*KV //  属性奖励
}
