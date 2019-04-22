
# TBO
# 三种顶级对象 @execl @type @print

## @execl 表示execl文件集合，支持各种文件路径，以及通配符多文件匹配

	格式：@execl 自定义数据集名 [execl文件路径 |...] 
	例子：@execl Fg ["*.xlsx" "[A-Z].*.xls"]	

## @type 数据类型 描述 sheet 单元中的的数据类型 可以多级嵌套 
	
	基础数据类型：
	
	int      整数
	uint     0-正无穷大 整数
	float    浮点
	string   字符串 例如 test
 	bytes    二进制串 'test'
    bool     true | false
	
	复合类型: 
    
    array 数组 数据使用()包括 元素值通过','分割
		 element 类型为any
		 例如：array(int|string|Equip) array(Equip.id)
		 数据： (2313, test, t132)
    map 字典 数据使用()包括 元素值通过','分割 key和value通过':' 分割
   	 	 key   类型为simple
		 value 类型为any 
		 例如：map(int|string:array(float|string|Equip)) *Equip 为定义类型Struct*
		 数据：(test:3232, 132:3233)
    struct 结构体 数据使用()包括 字段值通过'；'分割
		 必须定义@type赋予名字才能在类型中引用 定义各filed的name type ext 格式 name : type ``
		 filed 类型为any
		 ext 为 字段扩展属性 现在 支持输出器属性 erl lua hrl @printer
		 	 子属性 ignore 有的话表示忽略这个字段
		 	 子属性 default=v 指定默认值 
		 例如:
 		 @type Equip {
			id   : uint|string  
			name : string 		`erl:ignore lua:default=test`
			exp  : simple
			test : <GoodsID = int|string>|  这个字段被索引 
		 }
		 数据：(1313; 红杆)
	structIndex 可以指定字段索引 为结构体索引类型 只能用于array的元素
		 如array(Equip.id)
    set 集合类型
    	数据绑定sheet的行列 列支持嵌套组合
		四种 enum map array(struct.field) array(struct) 可以作为set的子类型, 前三种为带索引
	    enum 枚举
			 类型 只能 int uint string bytes 其中一种
			 绑定三列 名字，值，说明 名字为索引 类型为 string
			 例如：@type GameEnum <Fg.GameEnum[A B C][2.] = enum(int)>  
			 配置引用：GameEnum.Exp 或 Exp
		map 字典 
			 绑定两列 键，值 键为索引
			 例如：@type Global <Fg.Global[A B][2.] = map(int|string:int|string|Conf)>  
		array(struct.field) 数组
			 绑定与struct字段数相同的列 指定字段为索引 此字段类型为simple
			 例如：@type GoodsID <Fg.Goods[A.B [E.F] G][2.] = array(Goods.id)> 
		array(struct)
			 不带索引，不能用于index类型
	allset 集合并类型
		多个set的值并列
		格式: (set, set)
		例如: (GoodsID, EqAttr)
	index 索引类型
		<set|set = simple> 或者 <set|set>
		set为带索引的类型
		set的索引类型为 simple
		index 不指定匹配的simple索引类型 就使用set的索引类型作为匹配类型
		例如： <GoodsID = int>, <GoodsID>, array(<GoodsID = int>|<GoodsID>), map(int:<GoodsID>)
	*可通过@type 定义 struct set 类型*
	*上面类型可以称为单类型，还有两种特殊的类型可以称为多类型。 就是值同时可以是多个类型*

	多类型:

	any 同时包含所有单类型 一种以上可通过‘|’分隔 
 	simple 同时包含int uint string bytes 一到四种基础单类型 等价于 int|uint|string 可用于类型定义
 		多类型是有优先级的，默认规则是同类型从定义的顺序逐个匹配
 		set > index > int = uint = float > bool > string = bytes > array = map = struct = structIndex

## @print 把绑定类型的数据格式化输出到指定文件 已经实现输出器erl, hrl, esys, lua, xml

	格式 @print 输出类型,|... {
			输出器 输出符 文件地址 [扩展属性]
		}
	输出器
		erl  erlang数据结构 只输出 set(array(struct.field)|array(struct))
		hrl  erlang头文件的数据结构 只输出 set(enum) struct 继承erl
		esys erlang的sys.config 只通过~> 输出 set(map|array(struct.field)) ~引用set 转换成erlang的[{key,value}] 形式 !引用保留默认的set 继承erl
		lua  lua数据结构 输出 set(enum|map|array(struct.field)|array(struct))
		xml  xml数据结构 输出 set(enum|map|array(struct.field)|array(struct))
	输出类型
		set 作为顶级初始所有数据
		struct 结构信息 
	输出符
		>   覆盖文件输出
		>>  追加文件输出
		~>  格式化模板输出 只有esys支持
	扩展属性（为可选项)
		prefix       struct 名称前缀
		suffix       struct 名称后缀
		template     ~> 模板文件

	例如:
		@print ColorEnumBind, CoinEnumBind {
			lua > "tbo/example/global_def.lua"
			hrl > "tbo/example/fg_def.hrl"
		}

		@print Grids {
			xml > "tbo/example/grids.xml" `replaceChildType`
		}


## ext 说明
	扩展现在用于 strcut的field 和 print的输出器
	属性:
		ignore 对输出器忽略
		default 对于输出器字段默认值
		prefix suffix 输出器输出类型名时前后缀
		template 对于输出器操作为~> 的模板文件
		filter  过滤记录，只要字段的值在规则内整条记录会被忽略 多个可以用;隔开
		skipIndex 跳过输出索引字段，以非索引输出 
			值为all时为此字段定义的全部索引类型 个别多个可以用;隔开
		replaceChildType xml输出器使用字段名替换嵌套子类型名 

