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

            leftPane.style.width = `${newLeftWidth}px`
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