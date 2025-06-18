const methods = {
	initResizer(containerClass, leftPaneClass, rightPaneClass, resizerClass) {
        const resizer = document.querySelector(resizerClass)
        const leftPane = document.querySelector(leftPaneClass)
        const rightPane = document.querySelector(rightPaneClass)
        const container = document.querySelector(containerClass)

        let isResizing = false

        resizer.addEventListener('mousedown', (e) => {
            isResizing = true
            document.body.style.cursor = 'col-resize'
        })

        document.addEventListener('mousemove', (e) => {
            if (!isResizing) return

            const containerOffsetLeft = container.offsetLeft
            const newLeftWidth = e.clientX - containerOffsetLeft

            // Set minimum/maximum limits if desired
//            if (newLeftWidth > 300 && newLeftWidth < container.offsetWidth - 100) {
                leftPane.style.width = `${newLeftWidth}px`
console.log(leftPane.style.width)
                //            }
        })

        document.addEventListener('mouseup', () => {
            isResizing = false
            document.body.style.cursor = 'default'
        })
    }
}

export default {
	data () {
		return {
		}
	},
	methods: methods
}