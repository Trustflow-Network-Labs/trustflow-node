const methods = {
	copyToClipboard(event){
        const that = this
        const content = event.target.getAttribute('data-ref')
        if (!navigator.clipboard){
            this.$refs[content].focus()
            this.$refs[content].select()
            document.execCommand('copy')
        }
        else {
            navigator.clipboard.writeText(content).then(
                () => {
                    console.log('Copied!')
                })
                .catch(
                () => {
                    console.log('Error!')
            })
        }
    }
}

export default {
	data () {
		return {
		}
	},
	methods: methods
}
