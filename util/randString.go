package util

import (
    "fmt"
    "math/rand"
    "time"
    "strconv"
)

var rs = rand.New(rand.NewSource(time.Now().UnixNano()))
var EnumMap = make(map[string][]string)
var randConfig map[string][]string

type RandStruct struct{
    randslice []string
    index int
}

func (r *RandStruct) GetNext()(string){
    r.index++
    if(r.index >= len(r.randslice)){
        r.index = 0
    }
    return r.randslice[r.index]
}

//randConfig:初始化长度，最小长度，最大长度，模式(0:小写字母,1:大写字母,2:数字,3:字母+数字,4:大小写字母,5:汉字,6:大写开头的字母)
func InitRand(dataConfig *DataConfig) map[string]*RandStruct{
    randConfig = dataConfig.RandConfMap
    EnumMap = dataConfig.EnumlistMap

    fmt.Println(randConfig)

    var randValueMap = make(map[string]*RandStruct)

    for name,config := range randConfig{
        initsize,_ := strconv.Atoi(config[0])
        randValueMap[name]= &RandStruct{make([]string,initsize,initsize),-1};
        if(len(config) == 4){
            for i:=0;i< initsize; i++{
                randValueMap[name].randslice[i] = RandString(config[1],config[2],config[3]) 
            }
        }else{
            for i:=0;i< initsize; i++{
                randValueMap[name].randslice[i] = EnumString(config[1]) 
            }
        }

        //fmt.Println(randValueMap[name])
    }

    return randValueMap
}

//由于是采用的字符串相加的方式，效率不高，此工具采用的是事先初始化好一个不太长的随机串数组，需要用时从数组里循环取
func RandString(min string,max string, mod string) string{

    lowers := "abcdefghijklmnopqrstuvwxyz"
    uppers := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
    digits := "0123456789"
    alnums := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    alphas := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
    chinese := "的一是在不了有和人这中大为上个国我以要他时来用们生到作地于出就分对成会可主发年动同工也能下过子说产种面而方后多定行学法所民得经十三之进着等部度家电力里如水化高自二理起小物现实加量都两体制机当使点从业本去把性好应开它合还因由其些然前外天政四日那社义事平形相全表间样与关各重新线内数正心反你明看原又么利比或但质气第向道命此变条只没结解问意建月公无系军很情者最立代想已通并提直题党程展五果料象员革位入常文总次品式活设及管特件长求老头基资边流路级少图山统接知较将组见计别她手角期根论运农指几九区强放决西被干做必战先回则任取据处队南给色光门即保治北造百规热领七海口东导器压志世金增争济阶油思术极交受联什认六共权收证改清己美再采转更单风切打白教速花带安场身车例真务具万每目至达走积示议声报斗完类八离华名确才科张信马节话米整空元况今集温传土许步群广石记需段研界拉林律叫且究观越织装影算低持音众书布复容儿须际商非验连断深难近矿千周委素技备半办青省列习响约支般史感劳便团往酸历市克何除消构府称太准精值号率族维划选标写存候毛亲快效斯院查江型眼王按格养易置派层片始却专状育厂京识适属圆包火住调满县局照参红细引听该铁价严龙飞"

    dicts := map[string]string{"lowers":lowers,"uppers":uppers,"digits":digits,"alnums":alnums,"alphas":alphas,"chinese":chinese}

    min_size,_ := strconv.Atoi(min)
    max_size,_ := strconv.Atoi(max)

    result,n := "",min_size 
    if max_size > min_size {
        n = rs.Intn(max_size-min_size+1)+ min_size
    }

    switch {
    case mod == "lowers" || mod =="uppers" || mod =="digits" || mod =="alnums" || mod =="alphas":
        length := len(dicts[mod])
        for i:=0; i<n; i++{
            result = result + string(dicts[mod][rs.Intn(length)])
        }
    case mod == "chinese":
        length := len(chinese)
        for i:=0; i<n; i++{
            x := rs.Intn(length/3)   
            result = result + chinese[x*3:x*3+3]    //汉字在utf-8里是占用3个byte的
        }
    default :                                     //默认是首字母大写，后面字母小写的方式
        result = string(uppers[rs.Intn(26)])
        for i:=1; i<n; i++{
            result = result + string(lowers[rs.Intn(26)])
        }
    }

    return result
}

//由于
func EnumString(mod string) string{

    result := "" 

    Enumlist,ok := EnumMap[mod]
    if !ok{
        return result;
    }

    
    length := len(Enumlist)
    result = Enumlist[rs.Intn(length)]

    return result
}