package Time_Wheel

import (
	"container/list"
	"time"
)

func New(interval time.Duration, slotNum int, job Job) *TimeWheel {
	if interval <= 0 || slotNum <= 0 || job == nil {
		return nil
	}
	tw := &TimeWheel{
		interval:          interval,
		slots:             make([]*Slot, slotNum),
		timer:             make(map[interface{}]int),
		currentPos:        0,
		job:               job,
		slotNum:           slotNum,
		addTaskChannel:    make(chan Task),
		removeTaskChannel: make(chan interface{}),
		stopChannel:       make(chan bool),
	}

	tw.initSlots()

	return tw
}

func (tw *TimeWheel) initSlots() {
	for i := 0; i < tw.slotNum; i++ {
		tw.slots[i] = &Slot{
			list: list.New(),
		}
	}
}

func (tw *TimeWheel) Start() {
	tw.ticker = time.NewTicker(tw.interval)
	go tw.start()
}

func (tw *TimeWheel) Stop() {
	tw.stopChannel <- true
}

func (tw *TimeWheel) AddTimer(delay time.Duration, key interface{}, data interface{}) {
	if delay < 0 {
		return
	}
	tw.addTaskChannel <- Task{delay: delay, key: key, data: data}
}

func (tw *TimeWheel) RemoveTimer(key interface{}) {
	if key == nil {
		return
	}
	tw.removeTaskChannel <- key
}

func (tw *TimeWheel) start() {
	for {
		select {
		case <-tw.ticker.C:
			tw.tickHandler()
		case task := <-tw.addTaskChannel:
			tw.addTask(&task)
		case key := <-tw.removeTaskChannel:
			tw.removeTask(key)
		case <-tw.stopChannel:
			tw.ticker.Stop()
			return
		}
	}
}

func (tw *TimeWheel) tickHandler() {
	slot := tw.slots[tw.currentPos]
	tw.scanAndRunTask(slot)
	tw.currentPos = (tw.currentPos + 1) % tw.slotNum
}

func (tw *TimeWheel) scanAndRunTask(slot *Slot) {
	slot.lock.RLock()
	defer slot.lock.RUnlock()
	l := slot.list
	for e := l.Front(); e != nil; {
		task := e.Value.(*Task)
		if task.circle > 0 {
			// 13点 & 1点的数据是同一个list下，1点是一圈，13点是两圈
			task.circle--
			e = e.Next()
			continue
		}
		// 异步执行任务 & 从双链表中删除节点
		go tw.job(task.data)
		next := e.Next()
		l.Remove(e)
		if task.key != nil {
			delete(tw.timer, task.key)
		}
		e = next
	}
}

func (tw *TimeWheel) addTask(task *Task) {
	pos, circle := tw.getPositionAndCircle(task.delay)
	task.circle = circle

	slot := tw.slots[pos]
	slot.lock.Lock()
	defer slot.lock.Unlock()

	slot.list.PushBack(task)

	if task.key != nil {
		tw.timer[task.key] = pos
	}
}

// 获取定时器在槽中的位置, 时间轮需要转动的圈数
func (tw *TimeWheel) getPositionAndCircle(d time.Duration) (pos int, circle int) {
	delaySeconds := int(d.Seconds())
	intervalSeconds := int(tw.interval.Seconds())
	circle = delaySeconds / intervalSeconds / tw.slotNum
	pos = (tw.currentPos + delaySeconds/intervalSeconds) % tw.slotNum

	return
}

func (tw *TimeWheel) removeTask(key interface{}) {
	position, ok := tw.timer[key] // 获取定时器所在的槽
	if !ok {
		return
	}

	slot := tw.slots[position]
	slot.lock.RLock()
	defer slot.lock.RUnlock()
	l := slot.list
	for e := l.Front(); e != nil; {
		task := e.Value.(*Task)
		if task.key == key {
			delete(tw.timer, task.key)
			l.Remove(e)
			break
		}

		e = e.Next()
	}
}
