
-record('KVTmpl',
  {
    key             :: non_neg_integer(), %%  键
    value           :: non_neg_integer()|float() %%  值
  }).

-type 'KVTmpl'() :: #'KVTmpl'{}.

-record('UserTmpl',
  {
    level           :: non_neg_integer(), %%  等级
    maxEnergy       :: non_neg_integer(), %%  体力上限
    perexp          :: non_neg_integer(), %%  单次钓鱼经验
    exp             :: non_neg_integer(), %%  升级经验
    reward          :: ['KVTmpl'()], %%  升级奖励
    attrs           :: ['KVTmpl'()] %%  属性奖励
  }).

-type 'UserTmpl'() :: #'UserTmpl'{}.
