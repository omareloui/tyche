package com.omareloui.tyche

import android.content.Context
import android.util.AttributeSet
import android.view.View
import androidx.core.content.res.use
import androidx.core.widget.NestedScrollView

/** A NestedScrollView wrapping its content up to android:maxHeight, scrolling past it. */
class MaxHeightScrollView(context: Context, attrs: AttributeSet?) : NestedScrollView(context, attrs) {

    private val maxHeight: Int =
        context.obtainStyledAttributes(attrs, intArrayOf(android.R.attr.maxHeight)).use { it.getDimensionPixelSize(0, 0) }

    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        val spec = if (maxHeight > 0) View.MeasureSpec.makeMeasureSpec(maxHeight, View.MeasureSpec.AT_MOST) else heightMeasureSpec
        super.onMeasure(widthMeasureSpec, spec)
    }
}
